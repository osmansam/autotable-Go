package models

type PipelineStageInput struct {
    Match     string `json:"match,omitempty"`
    Lookup    string `json:"lookup,omitempty"`
    Unwind    string `json:"unwind,omitempty"`
    Group     string `json:"group,omitempty"`
    Sort      string `json:"sort,omitempty"`
    AddFields string `json:"addFields,omitempty"`
    Limit     string `json:"limit,omitempty"`
    Skip      string `json:"skip,omitempty"`
    Facet     string `json:"facet,omitempty"`
    Merge     string `json:"merge,omitempty"`
    Out       string `json:"out,omitempty"`
}
