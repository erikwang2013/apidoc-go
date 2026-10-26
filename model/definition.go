package model

// Definition declares an external protocol definition (OpenAPI, GraphQL, proto,
// ...). It is input metadata only: the legacy `/api` export strips it, while the
// UIC detail endpoint renders it.
type Definition struct {
	File           string `json:"file,omitempty"`
	Inline         string `json:"inline,omitempty"`
	Format         string `json:"format,omitempty"`
	Operation      string `json:"operation,omitempty"`
	Message        string `json:"message,omitempty"`
	Direction      string `json:"direction,omitempty"`
	Sample         string `json:"sample,omitempty"`
	SampleFile     string `json:"sampleFile,omitempty"`
	SampleEncoding string `json:"sampleEncoding,omitempty"`
	// Strict defaults to true; set false to downgrade unsupported syntax.
	Strict *bool `json:"strict,omitempty"`
}
