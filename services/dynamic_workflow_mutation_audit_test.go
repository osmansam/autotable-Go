package services

import (
	"math"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestWorkflowMutationAuditRejectsTypedBSONSelectorsAndModifiers(t *testing.T) {
	selector, err := bson.Marshal(bson.D{{Key: "$in", Value: bson.A{"admin"}}})
	if err != nil {
		t.Fatal(err)
	}
	modifiers, err := bson.Marshal(bson.D{{Key: "$each", Value: bson.A{"admin"}}, {Key: "$position", Value: 0}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		document map[string]interface{}
		update   bson.M
	}{
		{name: "pull BSON regex", document: map[string]interface{}{"items": bson.A{"admin", "viewer"}}, update: bson.M{"$pull": bson.M{"items": primitive.Regex{Pattern: "^admin$"}}}},
		{name: "pull raw document", document: map[string]interface{}{"items": bson.A{"admin", "viewer"}}, update: bson.M{"$pull": bson.M{"items": bson.Raw(selector)}}},
		{name: "pull raw document value", document: map[string]interface{}{"items": bson.A{"admin", "viewer"}}, update: bson.M{"$pull": bson.M{"items": bson.RawValue{Type: bsontype.EmbeddedDocument, Value: selector}}}},
		{name: "push raw document modifiers", document: map[string]interface{}{"items": bson.A{"viewer"}}, update: bson.M{"$push": bson.M{"items": bson.Raw(modifiers)}}},
		{name: "push raw document value modifiers", document: map[string]interface{}{"items": bson.A{"viewer"}}, update: bson.M{"$push": bson.M{"items": bson.RawValue{Type: bsontype.EmbeddedDocument, Value: modifiers}}}},
		{name: "increment BSON date", document: map[string]interface{}{"count": primitive.DateTime(1000)}, update: bson.M{"$inc": bson.M{"count": int64(1)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := applyWorkflowUpdateForAccess(tt.document, tt.update); err == nil {
				t.Fatalf("typed BSON operand with unsupported Mongo semantics was accepted: %#v", tt.update)
			}
		})
	}
}

func TestWorkflowMutationAuditPushPreservesScalarBSONOperands(t *testing.T) {
	for name, value := range map[string]interface{}{
		"ObjectID": primitive.NewObjectID(),
		"date":     primitive.DateTime(1000),
		"binary":   primitive.Binary{Subtype: 0x80, Data: []byte{1, 2}},
		"regex":    primitive.Regex{Pattern: "^admin$", Options: "i"},
	} {
		t.Run(name, func(t *testing.T) {
			document := map[string]interface{}{}
			if err := applyWorkflowUpdateForAccess(document, bson.M{"$push": bson.M{"items": value}}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(document["items"], []interface{}{value}) {
				t.Fatalf("scalar BSON operand was changed: %#v", document)
			}
		})
	}
}

func TestWorkflowMutationAuditRejectsUnsimulatedUpdates(t *testing.T) {
	tests := []struct {
		name     string
		document map[string]interface{}
		update   bson.M
	}{
		{name: "push position", document: map[string]interface{}{"items": []interface{}{1, 2}}, update: bson.M{"$push": bson.M{"items": bson.M{"$each": []interface{}{3}, "$position": 0}}}},
		{name: "push slice", document: map[string]interface{}{"items": []interface{}{1, 2}}, update: bson.M{"$push": bson.M{"items": bson.M{"$each": []interface{}{3}, "$slice": 1}}}},
		{name: "push sort", document: map[string]interface{}{"items": []interface{}{2, 1}}, update: bson.M{"$push": bson.M{"items": bson.M{"$each": []interface{}{3}, "$sort": 1}}}},
		{name: "push invalid each", document: map[string]interface{}{}, update: bson.M{"$push": bson.M{"items": bson.M{"$each": "bad"}}}},
		{name: "addToSet invalid each", document: map[string]interface{}{}, update: bson.M{"$addToSet": bson.M{"items": bson.M{"$each": 1}}}},
		{name: "addToSet extra modifier", document: map[string]interface{}{}, update: bson.M{"$addToSet": bson.M{"items": bson.M{"$each": []interface{}{1}, "$slice": 1}}}},
		{name: "pull in selector", document: map[string]interface{}{"items": []interface{}{1, 2}}, update: bson.M{"$pull": bson.M{"items": bson.M{"$in": []interface{}{1}}}}},
		{name: "pull comparison selector", document: map[string]interface{}{"items": []interface{}{1, 2}}, update: bson.M{"$pull": bson.M{"items": bson.M{"$gte": 2}}}},
		{name: "pull document selector", document: map[string]interface{}{"items": []interface{}{bson.M{"owner": "alice", "role": "admin"}}}, update: bson.M{"$pull": bson.M{"items": bson.M{"owner": "alice"}}}},
		{name: "pull ordered selector", document: map[string]interface{}{"items": []interface{}{1}}, update: bson.M{"$pull": bson.M{"items": bson.D{{Key: "$eq", Value: 1}}}}},
		{name: "push nonarray target", document: map[string]interface{}{"items": "text"}, update: bson.M{"$push": bson.M{"items": 1}}},
		{name: "push null target", document: map[string]interface{}{"items": nil}, update: bson.M{"$push": bson.M{"items": 1}}},
		{name: "addToSet nonarray target", document: map[string]interface{}{"items": 1}, update: bson.M{"$addToSet": bson.M{"items": 2}}},
		{name: "pull nonarray target", document: map[string]interface{}{"items": "text"}, update: bson.M{"$pull": bson.M{"items": 1}}},
		{name: "pullAll nonarray target", document: map[string]interface{}{"items": "text"}, update: bson.M{"$pullAll": bson.M{"items": []interface{}{1}}}},
		{name: "pullAll operand not array", document: map[string]interface{}{"items": []interface{}{1}}, update: bson.M{"$pullAll": bson.M{"items": 1}}},
		{name: "inc null target", document: map[string]interface{}{"count": nil}, update: bson.M{"$inc": bson.M{"count": 1}}},
		{name: "inc string target", document: map[string]interface{}{"count": "1"}, update: bson.M{"$inc": bson.M{"count": 1}}},
		{name: "inc string operand", document: map[string]interface{}{"count": 1}, update: bson.M{"$inc": bson.M{"count": "1"}}},
		{name: "inc nonfinite operand", document: map[string]interface{}{"count": 1}, update: bson.M{"$inc": bson.M{"count": math.Inf(1)}}},
		{name: "inc integer overflow", document: map[string]interface{}{"count": int64(math.MaxInt64)}, update: bson.M{"$inc": bson.M{"count": int64(1)}}},
		{name: "push typed null array", document: map[string]interface{}{"items": primitive.A(nil)}, update: bson.M{"$push": bson.M{"items": 1}}},
		{name: "push ObjectID target", document: map[string]interface{}{"items": primitive.NewObjectID()}, update: bson.M{"$push": bson.M{"items": 1}}},
		{name: "push binary target", document: map[string]interface{}{"items": []byte{1, 2}}, update: bson.M{"$push": bson.M{"items": 1}}},
		{name: "push binary each", document: map[string]interface{}{}, update: bson.M{"$push": bson.M{"items": bson.M{"$each": []byte{1, 2}}}}},
		{name: "push null each array", document: map[string]interface{}{}, update: bson.M{"$push": bson.M{"items": bson.M{"$each": primitive.A(nil)}}}},
		{name: "addToSet unordered document equality", document: map[string]interface{}{"items": []interface{}{bson.M{"a": 1, "b": 2}}}, update: bson.M{"$addToSet": bson.M{"items": bson.M{"a": 1, "b": 2}}}},
		{name: "addToSet ordered document equality", document: map[string]interface{}{"items": []interface{}{bson.M{"a": 1}}}, update: bson.M{"$addToSet": bson.M{"items": bson.D{{Key: "a", Value: 1}}}}},
		{name: "unknown operator", document: map[string]interface{}{}, update: bson.M{"$rename": bson.M{"a": "b"}}},
		{name: "unknown empty operator", document: map[string]interface{}{}, update: bson.M{"$rename": bson.M{}}},
		{name: "nonobject fields", document: map[string]interface{}{}, update: bson.M{"$set": "bad"}},
		{name: "empty path", document: map[string]interface{}{}, update: bson.M{"$unset": bson.M{"": 1}}},
		{name: "dotted invalid path", document: map[string]interface{}{}, update: bson.M{"$unset": bson.M{"a..b": 1}}},
		{name: "operator path", document: map[string]interface{}{}, update: bson.M{"$unset": bson.M{"a.$bad": 1}}},
		{name: "trimmed path differs from database", document: map[string]interface{}{}, update: bson.M{"$set": bson.M{" a ": 1}}},
		{name: "unset array index unsupported", document: map[string]interface{}{"items": []interface{}{1, 2}}, update: bson.M{"$unset": bson.M{"items.0": 1}}},
		{name: "conflicting paths", document: map[string]interface{}{}, update: bson.M{"$set": bson.M{"profile": bson.M{}, "profile.name": "alice"}}},
		{name: "conflicting operators", document: map[string]interface{}{}, update: bson.M{"$set": bson.M{"count": 1}, "$inc": bson.M{"count": 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := applyWorkflowUpdateForAccess(tt.document, tt.update); err == nil {
				t.Fatalf("update whose final record cannot be simulated safely was accepted: %#v", tt.update)
			}
		})
	}
}

func TestWorkflowMutationAuditSupportedOperators(t *testing.T) {
	tests := []struct {
		name     string
		document map[string]interface{}
		update   bson.M
		want     map[string]interface{}
	}{
		{name: "set creates nested object", document: map[string]interface{}{}, update: bson.M{"$set": bson.M{"profile.name": "alice"}}, want: map[string]interface{}{"profile": map[string]interface{}{"name": "alice"}}},
		{name: "set retains other fields", document: map[string]interface{}{"keep": true, "name": "alice"}, update: bson.M{"$set": bson.M{"name": "bob"}}, want: map[string]interface{}{"keep": true, "name": "bob"}},
		{name: "unset existing", document: map[string]interface{}{"keep": true, "remove": 1}, update: bson.M{"$unset": bson.M{"remove": ""}}, want: map[string]interface{}{"keep": true}},
		{name: "unset missing parent", document: map[string]interface{}{"keep": true}, update: bson.M{"$unset": bson.M{"missing.field": ""}}, want: map[string]interface{}{"keep": true}},
		{name: "unset scalar parent", document: map[string]interface{}{"parent": 1}, update: bson.M{"$unset": bson.M{"parent.field": ""}}, want: map[string]interface{}{"parent": 1}},
		{name: "inc existing int32", document: map[string]interface{}{"count": int32(1)}, update: bson.M{"$inc": bson.M{"count": int32(2)}}, want: map[string]interface{}{"count": int32(3)}},
		{name: "inc int32 promotes on overflow", document: map[string]interface{}{"count": int32(math.MaxInt32)}, update: bson.M{"$inc": bson.M{"count": int32(1)}}, want: map[string]interface{}{"count": int64(math.MaxInt32) + 1}},
		{name: "inc missing", document: map[string]interface{}{}, update: bson.M{"$inc": bson.M{"count": int64(2)}}, want: map[string]interface{}{"count": int64(2)}},
		{name: "inc negative", document: map[string]interface{}{"count": int64(2)}, update: bson.M{"$inc": bson.M{"count": int64(-3)}}, want: map[string]interface{}{"count": int64(-1)}},
		{name: "inc fractional", document: map[string]interface{}{"count": int64(1)}, update: bson.M{"$inc": bson.M{"count": 0.5}}, want: map[string]interface{}{"count": 1.5}},
		{name: "push duplicates", document: map[string]interface{}{"items": primitive.A{1}}, update: bson.M{"$push": bson.M{"items": 1}}, want: map[string]interface{}{"items": []interface{}{1, 1}}},
		{name: "push each", document: map[string]interface{}{"items": []interface{}{1}}, update: bson.M{"$push": bson.M{"items": bson.M{"$each": primitive.A{2, 3}}}}, want: map[string]interface{}{"items": []interface{}{1, 2, 3}}},
		{name: "push array as one item", document: map[string]interface{}{}, update: bson.M{"$push": bson.M{"items": []interface{}{1, 2}}}, want: map[string]interface{}{"items": []interface{}{[]interface{}{1, 2}}}},
		{name: "push empty each creates array", document: map[string]interface{}{}, update: bson.M{"$push": bson.M{"items": bson.M{"$each": []interface{}{}}}}, want: map[string]interface{}{"items": []interface{}{}}},
		{name: "addToSet deduplicates each", document: map[string]interface{}{"items": []interface{}{1}}, update: bson.M{"$addToSet": bson.M{"items": bson.M{"$each": []interface{}{1, 2, 2}}}}, want: map[string]interface{}{"items": []interface{}{1, 2}}},
		{name: "addToSet distinct document", document: map[string]interface{}{"items": []interface{}{bson.M{"name": "alice"}}}, update: bson.M{"$addToSet": bson.M{"items": bson.M{"name": "bob"}}}, want: map[string]interface{}{"items": []interface{}{bson.M{"name": "alice"}, bson.M{"name": "bob"}}}},
		{name: "pull all scalar matches", document: map[string]interface{}{"items": []interface{}{1, 2, 1}}, update: bson.M{"$pull": bson.M{"items": 1}}, want: map[string]interface{}{"items": []interface{}{2}}},
		{name: "pullAll scalar matches", document: map[string]interface{}{"items": []interface{}{1, 2, 3}}, update: bson.M{"$pullAll": bson.M{"items": []interface{}{1, 3}}}, want: map[string]interface{}{"items": []interface{}{2}}},
		{name: "setOnInsert leaves existing record", document: map[string]interface{}{"name": "alice"}, update: bson.M{"$setOnInsert": bson.M{"name": "bob"}}, want: map[string]interface{}{"name": "alice"}},
		{name: "empty supported operator", document: map[string]interface{}{"keep": true}, update: bson.M{"$set": bson.M{}}, want: map[string]interface{}{"keep": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := applyWorkflowUpdateForAccess(tt.document, tt.update); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tt.document, tt.want) {
				t.Fatalf("final document = %#v, want %#v", tt.document, tt.want)
			}
		})
	}
}

func TestWorkflowMutationAuditInvalidSetPathPreservesDocument(t *testing.T) {
	for _, path := range []string{"", ".name", "profile..name", "profile.$value", "profile.name.", "profile.\x00name"} {
		t.Run(path, func(t *testing.T) {
			document := map[string]interface{}{"keep": true}
			if err := workflowSetPath(document, path, "alice"); err == nil {
				t.Fatalf("invalid path %q was accepted", path)
			}
			if !reflect.DeepEqual(document, map[string]interface{}{"keep": true}) {
				t.Fatalf("invalid path partially mutated document: %#v", document)
			}
		})
	}
}

func TestWorkflowMutationAuditScalarEqualityPreservesMongoTypes(t *testing.T) {
	tests := []struct {
		name   string
		before []interface{}
		update bson.M
		want   []interface{}
	}{
		{name: "pull number retains numeric string", before: []interface{}{"1", int32(1), "other"}, update: bson.M{"$pull": bson.M{"items": int64(1)}}, want: []interface{}{"1", "other"}},
		{name: "pullAll number retains numeric string", before: []interface{}{"1", int32(1), "other"}, update: bson.M{"$pullAll": bson.M{"items": []interface{}{int64(1)}}}, want: []interface{}{"1", "other"}},
		{name: "addToSet number differs from numeric string", before: []interface{}{"1"}, update: bson.M{"$addToSet": bson.M{"items": int32(1)}}, want: []interface{}{"1", int32(1)}},
		{name: "addToSet equal numeric types", before: []interface{}{int32(1)}, update: bson.M{"$addToSet": bson.M{"items": int64(1)}}, want: []interface{}{int32(1)}},
		{name: "pull large integer compares exactly", before: []interface{}{int64(9007199254740992), int64(9007199254740993)}, update: bson.M{"$pull": bson.M{"items": int64(9007199254740993)}}, want: []interface{}{int64(9007199254740992)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := map[string]interface{}{"items": tt.before}
			if err := applyWorkflowUpdateForAccess(document, tt.update); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(document["items"], tt.want) {
				t.Fatalf("items = %#v, want %#v", document["items"], tt.want)
			}
		})
	}
}

func TestWorkflowMutationAuditIncrementPreservesIntegerPrecision(t *testing.T) {
	document := map[string]interface{}{"count": int64(9007199254740992)}
	if err := applyWorkflowUpdateForAccess(document, bson.M{"$inc": bson.M{"count": int64(1)}}); err != nil {
		t.Fatal(err)
	}
	if document["count"] != int64(9007199254740993) {
		t.Fatalf("count = %#v, want exact int64 9007199254740993", document["count"])
	}
}

func TestWorkflowMutationAuditPullMissingFieldIsNoop(t *testing.T) {
	for _, operator := range []string{"$pull", "$pullAll"} {
		t.Run(operator, func(t *testing.T) {
			document := map[string]interface{}{"keep": true}
			var value interface{} = 1
			if operator == "$pullAll" {
				value = []interface{}{1}
			}
			if err := applyWorkflowUpdateForAccess(document, bson.M{operator: bson.M{"missing": value}}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(document, map[string]interface{}{"keep": true}) {
				t.Fatalf("removing from a missing array created a field: %#v", document)
			}
		})
	}
}

func TestWorkflowMutationAuditBSONNestedPaths(t *testing.T) {
	for _, operator := range []string{"$set", "$unset"} {
		t.Run(operator, func(t *testing.T) {
			document := map[string]interface{}{"profile": bson.M{"name": "alice", "keep": true}}
			if err := applyWorkflowUpdateForAccess(document, bson.M{operator: bson.M{"profile.name": "bob"}}); err != nil {
				t.Fatal(err)
			}
			value, exists := workflowPathValue(document, "profile.name")
			if operator == "$set" && (!exists || value != "bob") {
				t.Fatalf("nested BSON set = %#v", document)
			}
			if operator == "$unset" && exists {
				t.Fatalf("nested BSON unset retained name: %#v", document)
			}
		})
	}
}

func TestWorkflowMutationAuditSetPathNilTargetReturnsError(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Errorf("nil target panicked instead of returning an error: %v", recovered)
		}
	}()
	if err := workflowSetPath(nil, "value", true); err == nil {
		t.Fatal("nil target must return an error")
	}
}

func TestWorkflowMutationAuditSetValueIsDetached(t *testing.T) {
	value := []bson.M{{"name": "alice"}}
	document := map[string]interface{}{}
	if err := applyWorkflowUpdateForAccess(document, bson.M{"$set": bson.M{"items": value}}); err != nil {
		t.Fatal(err)
	}
	document["items"].([]bson.M)[0]["name"] = "bob"
	if value[0]["name"] != "alice" {
		t.Fatalf("simulated document aliases the update payload: %#v", value)
	}
}

func TestWorkflowMutationAuditObjectIDEquality(t *testing.T) {
	id := primitive.NewObjectID()
	document := map[string]interface{}{"items": []interface{}{id, id.Hex()}}
	if err := applyWorkflowUpdateForAccess(document, bson.M{"$pull": bson.M{"items": id}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(document["items"], []interface{}{id.Hex()}) {
		t.Fatalf("ObjectID equality must preserve string values: %#v", document)
	}
}

func TestWorkflowMutationAuditFinalRecordAuthorizationUsesDetachedPreview(t *testing.T) {
	container, userID := workflowAccessFixture()
	source := map[string]interface{}{"ownerId": userID, "profile": bson.M{"name": "alice"}}
	finalDocument := cloneWorkflowMap(source)
	if err := applyWorkflowUpdateForAccess(finalDocument, bson.M{"$set": bson.M{"profile.name": "bob"}, "$unset": bson.M{"ownerId": ""}}); err != nil {
		t.Fatal(err)
	}
	if err := authorizeFinalUpdateRecord(container, container.Routes.UpdateDynamicModelItem.Access, callerAccessContext(userID.Hex()), finalDocument); err == nil {
		t.Fatal("final-record authorization accepted removal of the authorized owner")
	}
	if source["ownerId"] != userID || source["profile"].(bson.M)["name"] != "alice" {
		t.Fatalf("authorization preview mutated the source record: %#v", source)
	}
}
