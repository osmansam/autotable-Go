package services

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type fakeLocalizationJobRepository struct {
	entries          []models.TranslationEntry
	createdJob       *models.TranslationJob
	leasedJobs       []*models.TranslationJob
	storedKeys       []string
	failedKeys       []string
	manualRaceKeys   map[string]bool
	total            int64
	completed        int64
	skippedManual    int64
	failed           int64
	errorSummary     []string
	completeCallMade bool
}

func (f *fakeLocalizationJobRepository) CreateJob(_ context.Context, job *models.TranslationJob) error {
	copy := *job
	f.createdJob = &copy
	return nil
}

func (f *fakeLocalizationJobRepository) ListProjectTranslations(context.Context, primitive.ObjectID, primitive.ObjectID) ([]models.TranslationEntry, error) {
	return append([]models.TranslationEntry(nil), f.entries...), nil
}

func (f *fakeLocalizationJobRepository) SetJobRunning(_ context.Context, _ primitive.ObjectID, total int64) error {
	f.total = total
	return nil
}

func (f *fakeLocalizationJobRepository) UpdateJobProgress(_ context.Context, _ primitive.ObjectID, completed, skippedManual, failed int64, messages []string) error {
	f.completed, f.skippedManual, f.failed = completed, skippedManual, failed
	f.errorSummary = append([]string(nil), messages...)
	return nil
}

func (f *fakeLocalizationJobRepository) CompleteJob(_ context.Context, _ primitive.ObjectID, completed, skippedManual, failed int64, messages []string) error {
	f.completeCallMade = true
	f.completed, f.skippedManual, f.failed = completed, skippedManual, failed
	f.errorSummary = append([]string(nil), messages...)
	return nil
}

func (f *fakeLocalizationJobRepository) UpsertGeneratedTranslation(_ context.Context, entry models.TranslationEntry) (bool, error) {
	if f.manualRaceKeys[entry.TranslationKey] {
		return false, nil
	}
	f.storedKeys = append(f.storedKeys, entry.TranslationKey)
	return true, nil
}

func (f *fakeLocalizationJobRepository) MarkGeneratedTranslationFailed(_ context.Context, entry models.TranslationEntry) error {
	if !f.manualRaceKeys[entry.TranslationKey] {
		f.failedKeys = append(f.failedKeys, entry.TranslationKey)
	}
	return nil
}

func (f *fakeLocalizationJobRepository) ApplyReconciledEntries(_ context.Context, entries []models.TranslationEntry) error {
	for _, entry := range entries {
		if entry.Status == models.TranslationStatusFailed {
			f.failedKeys = append(f.failedKeys, entry.TranslationKey)
		}
	}
	return nil
}

func (f *fakeLocalizationJobRepository) LeaseNextJob(_ context.Context, owner string, _ time.Time, _ time.Duration) (*models.TranslationJob, error) {
	if len(f.leasedJobs) == 0 {
		return nil, mongo.ErrNoDocuments
	}
	job := f.leasedJobs[0]
	job.LeaseOwner = owner
	job.LeaseToken = "fake-claim-" + job.ID.Hex()
	f.leasedJobs = f.leasedJobs[1:]
	return job, nil
}

type fakeProjectLocalizationRepository struct {
	project      *models.Project
	increments   int
	incrementErr error
}

func (f *fakeProjectLocalizationRepository) GetProject(context.Context, primitive.ObjectID, primitive.ObjectID) (*models.Project, error) {
	if f.project == nil {
		return nil, mongo.ErrNoDocuments
	}
	copy := *f.project
	return &copy, nil
}

func (f *fakeProjectLocalizationRepository) IncrementVersion(context.Context, primitive.ObjectID, primitive.ObjectID) error {
	f.increments++
	return f.incrementErr
}

func TestProjectTranslationJobDoesNotWriteWhenCacheVersionCannotBeReserved(t *testing.T) {
	tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
	jobs := &fakeLocalizationJobRepository{entries: []models.TranslationEntry{{
		TenantID: tenantID, ProjectID: projectID, Locale: "tr", TranslationKey: "title", SourceText: "Title",
		SourceHash: "hash", Status: models.TranslationStatusMissing, Origin: models.TranslationOriginUntranslated, IsActive: true,
	}}}
	projects := &fakeProjectLocalizationRepository{
		project:      &models.Project{SourceLocale: "en", EnabledLocales: []string{"en", "tr"}},
		incrementErr: mongo.CommandError{Code: 91, Message: "version store unavailable"},
	}
	withLocalizationJobDependencies(t, jobs, projects)
	translateLocalizationBatch = func(context.Context, string, string, []models.SourceString) ([]TranslationOutput, error) {
		t.Fatal("translation provider must not run before cache version is reserved")
		return nil, nil
	}

	err := ProcessProjectTranslationJob(context.Background(), &models.TranslationJob{
		ID: primitive.NewObjectID(), TenantID: tenantID, ProjectID: projectID, SourceLocale: "en", TargetLocales: []string{"tr"},
	})
	if err == nil || !strings.Contains(err.Error(), "reserve localization version") {
		t.Fatalf("error = %v", err)
	}
	if len(jobs.storedKeys) != 0 || !jobs.completeCallMade || jobs.failed != 1 {
		t.Fatalf("stored=%#v completed=%v failed=%d", jobs.storedKeys, jobs.completeCallMade, jobs.failed)
	}
}

func withLocalizationJobDependencies(t *testing.T, jobs *fakeLocalizationJobRepository, projects *fakeProjectLocalizationRepository) {
	t.Helper()
	oldJobs := localizationJobRepositoryFactory
	oldProjects := projectLocalizationRepositoryFactory
	oldReconcile := reconcileTranslationsForJob
	oldTranslate := translateLocalizationBatch
	localizationJobRepositoryFactory = func() localizationJobRepository { return jobs }
	projectLocalizationRepositoryFactory = func() projectLocalizationRepository { return projects }
	reconcileTranslationsForJob = func(context.Context, primitive.ObjectID, primitive.ObjectID, string, []string) (models.TranslationReconciliationSummary, error) {
		return models.TranslationReconciliationSummary{}, nil
	}
	t.Cleanup(func() {
		localizationJobRepositoryFactory = oldJobs
		projectLocalizationRepositoryFactory = oldProjects
		reconcileTranslationsForJob = oldReconcile
		translateLocalizationBatch = oldTranslate
	})
}

func TestProjectTranslationJobQueuePersistsSourceLocaleAndKeyFilter(t *testing.T) {
	tenantID, projectID, userID := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	jobs := &fakeLocalizationJobRepository{}
	projects := &fakeProjectLocalizationRepository{project: &models.Project{
		ID: projectID, TenantID: tenantID, SourceLocale: "en", EnabledLocales: []string{"en", "tr", "de"},
	}}
	withLocalizationJobDependencies(t, jobs, projects)

	jobID, err := QueueProjectTranslationJob(tenantID, projectID, userID, "en", []string{"tr"}, []string{"b", "a", "a"})
	if err != nil {
		t.Fatal(err)
	}
	if jobs.createdJob == nil || jobs.createdJob.ID != jobID || jobs.createdJob.SourceLocale != "en" {
		t.Fatalf("created job = %#v", jobs.createdJob)
	}
	if !reflect.DeepEqual(jobs.createdJob.TranslationKeys, []string{"a", "b"}) {
		t.Fatalf("translation keys = %#v", jobs.createdJob.TranslationKeys)
	}
	if jobs.createdJob.Status != models.TranslationJobPending {
		t.Fatalf("status = %q", jobs.createdJob.Status)
	}
}

func TestProjectTranslationJobQueueRejectsDisabledLocale(t *testing.T) {
	jobs := &fakeLocalizationJobRepository{}
	projects := &fakeProjectLocalizationRepository{project: &models.Project{SourceLocale: "en", EnabledLocales: []string{"en", "tr"}}}
	withLocalizationJobDependencies(t, jobs, projects)

	_, err := QueueProjectTranslationJob(primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), "en", []string{"de"}, nil)
	if err == nil || !strings.Contains(err.Error(), "enabled") {
		t.Fatalf("disabled locale error = %v", err)
	}
	if jobs.createdJob != nil {
		t.Fatal("disabled-locale job was persisted")
	}
}

func TestProjectTranslationJobSelectsEligibleEntriesAndCountsManualSkips(t *testing.T) {
	tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
	entry := func(key string, origin models.TranslationOrigin, status models.TranslationStatus, active bool) models.TranslationEntry {
		return models.TranslationEntry{TenantID: tenantID, ProjectID: projectID, Locale: "tr", TranslationKey: key, ResourceType: "sitePages", ResourceID: "home", PropertyPath: key, SourceText: "Source " + key, SourceHash: "hash-" + key, TranslatedText: "text", Origin: origin, Status: status, IsActive: active}
	}
	jobs := &fakeLocalizationJobRepository{entries: []models.TranslationEntry{
		entry("missing", models.TranslationOriginUntranslated, models.TranslationStatusMissing, true),
		entry("outdated-ai", models.TranslationOriginAI, models.TranslationStatusOutdated, true),
		entry("failed", models.TranslationOriginAI, models.TranslationStatusFailed, true),
		entry("manual", models.TranslationOriginManual, models.TranslationStatusCurrent, true),
		entry("current-ai", models.TranslationOriginAI, models.TranslationStatusCurrent, true),
		entry("inactive", models.TranslationOriginAI, models.TranslationStatusFailed, false),
	}}
	projects := &fakeProjectLocalizationRepository{project: &models.Project{ID: projectID, TenantID: tenantID, SourceLocale: "en", EnabledLocales: []string{"en", "tr"}}}
	withLocalizationJobDependencies(t, jobs, projects)
	var translatedKeys []string
	translateLocalizationBatch = func(_ context.Context, _, _ string, items []models.SourceString) ([]TranslationOutput, error) {
		outputs := make([]TranslationOutput, 0, len(items))
		for _, item := range items {
			translatedKeys = append(translatedKeys, item.TranslationKey)
			outputs = append(outputs, TranslationOutput{Key: item.TranslationKey, Text: "translated " + item.TranslationKey})
		}
		return outputs, nil
	}
	job := &models.TranslationJob{ID: primitive.NewObjectID(), TenantID: tenantID, ProjectID: projectID, SourceLocale: "en", TargetLocales: []string{"tr"}}

	if err := ProcessProjectTranslationJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	sort.Strings(translatedKeys)
	if !reflect.DeepEqual(translatedKeys, []string{"failed", "missing", "outdated-ai"}) {
		t.Fatalf("translated keys = %#v", translatedKeys)
	}
	if jobs.total != 4 || jobs.completed != 3 || jobs.skippedManual != 1 || jobs.failed != 0 || !jobs.completeCallMade {
		t.Fatalf("job progress = total %d, completed %d, skipped %d, failed %d", jobs.total, jobs.completed, jobs.skippedManual, jobs.failed)
	}
	if projects.increments != 2 {
		t.Fatalf("version increments = %d", projects.increments)
	}
}

func TestProjectTranslationJobKeyFilterAndManualRace(t *testing.T) {
	tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
	jobs := &fakeLocalizationJobRepository{
		entries: []models.TranslationEntry{
			{TenantID: tenantID, ProjectID: projectID, Locale: "tr", TranslationKey: "selected", SourceText: "Selected", SourceHash: "one", Status: models.TranslationStatusMissing, Origin: models.TranslationOriginUntranslated, IsActive: true},
			{TenantID: tenantID, ProjectID: projectID, Locale: "tr", TranslationKey: "ignored", SourceText: "Ignored", SourceHash: "two", Status: models.TranslationStatusMissing, Origin: models.TranslationOriginUntranslated, IsActive: true},
		},
		manualRaceKeys: map[string]bool{"selected": true},
	}
	projects := &fakeProjectLocalizationRepository{project: &models.Project{SourceLocale: "en", EnabledLocales: []string{"en", "tr"}}}
	withLocalizationJobDependencies(t, jobs, projects)
	translateLocalizationBatch = func(_ context.Context, _, _ string, items []models.SourceString) ([]TranslationOutput, error) {
		return []TranslationOutput{{Key: items[0].TranslationKey, Text: "Çeviri"}}, nil
	}
	job := &models.TranslationJob{ID: primitive.NewObjectID(), TenantID: tenantID, ProjectID: projectID, SourceLocale: "en", TargetLocales: []string{"tr"}, TranslationKeys: []string{"selected"}}

	if err := ProcessProjectTranslationJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if jobs.completed != 0 || jobs.skippedManual != 1 || len(jobs.storedKeys) != 0 || projects.increments != 1 {
		t.Fatalf("manual race progress = completed %d skipped %d stored %#v increments %d", jobs.completed, jobs.skippedManual, jobs.storedKeys, projects.increments)
	}
}

func TestProjectTranslationJobProtectedTokenFailureAffectsOnlyEntry(t *testing.T) {
	tenantID, projectID := primitive.NewObjectID(), primitive.NewObjectID()
	jobs := &fakeLocalizationJobRepository{entries: []models.TranslationEntry{
		{TenantID: tenantID, ProjectID: projectID, Locale: "tr", TranslationKey: "bad", SourceText: "Hello {{name}}", SourceHash: "one", Status: models.TranslationStatusMissing, Origin: models.TranslationOriginUntranslated, IsActive: true},
		{TenantID: tenantID, ProjectID: projectID, Locale: "tr", TranslationKey: "good", SourceText: "Visit https://example.com", SourceHash: "two", Status: models.TranslationStatusMissing, Origin: models.TranslationOriginUntranslated, IsActive: true},
	}}
	projects := &fakeProjectLocalizationRepository{project: &models.Project{SourceLocale: "en", EnabledLocales: []string{"en", "tr"}}}
	withLocalizationJobDependencies(t, jobs, projects)
	translateLocalizationBatch = func(context.Context, string, string, []models.SourceString) ([]TranslationOutput, error) {
		return []TranslationOutput{
			{Key: "bad", Text: "Merhaba {{isim}}"},
			{Key: "good", Text: "https://example.com adresini ziyaret edin"},
		}, nil
	}
	job := &models.TranslationJob{ID: primitive.NewObjectID(), TenantID: tenantID, ProjectID: projectID, SourceLocale: "en", TargetLocales: []string{"tr"}}

	if err := ProcessProjectTranslationJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(jobs.storedKeys, []string{"good"}) || !reflect.DeepEqual(jobs.failedKeys, []string{"bad"}) {
		t.Fatalf("stored/failed = %#v/%#v", jobs.storedKeys, jobs.failedKeys)
	}
	if jobs.completed != 1 || jobs.failed != 1 || projects.increments != 2 {
		t.Fatalf("progress = completed %d failed %d increments %d", jobs.completed, jobs.failed, projects.increments)
	}
	if len(jobs.errorSummary) != 1 || !strings.Contains(jobs.errorSummary[0], "protected") {
		t.Fatalf("errors = %#v", jobs.errorSummary)
	}
}

func (f *fakeLocalizationJobRepository) RenewJobLease(context.Context, *models.TranslationJob, time.Time, time.Duration) (bool, error) {
	return true, nil
}
