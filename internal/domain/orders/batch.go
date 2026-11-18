// Package orders defines the shared order ingestion contract and merge rules.
// It has no dependency on external services, HTTP, filesystem or SQL.
package orders

type Fields = map[string]any

// Record identifies a package independently of the acquisition channel. Fields
// retains the existing read-model schema and supplemental fields losslessly;
// transport envelopes, authentication and pagination never enter this model.
type Record struct {
	ID     string
	Fields Fields
}

type Scope struct {
	Name       string
	CapturedAt string
	Files      []string
	PackageIDs []string
}

type ImportPolicy struct {
	PreserveMissingFields     bool
	RequireUnchangedAccount   bool
	RequireWholeOrderPurchase bool
}

type Batch struct {
	Source     string
	CapturedAt string
	Archive    string
	Records    []Record
	Scopes     []Scope
	Policy     ImportPolicy
}

type ImportResult struct {
	Packages           int `json:"packages"`
	WaitPurchase       int `json:"waitPurchase"`
	HasPurchase        int `json:"hasPurchase"`
	MissingRefundLines int `json:"missingRefundLines"`
	SupplementedLines  int `json:"supplementedLines"`
}

// Archive is a verified input document, independent of a provider's endpoint
// names, envelope keys and archive representation.
type Archive struct {
	Source     string
	Kind       Kind
	CapturedAt string
	Records    []Record
	Scopes     []Scope
}
type Kind string

const (
	Packages     Kind = "packages"
	Translations Kind = "translations"
	Products     Kind = "products"
	Outbound     Kind = "outbound"
)

// ArchiveDecoder is implemented by each capture format. The repository supplies
// the archive reader; adapters must verify complete scope evidence before return.
type ArchiveDecoder interface {
	DecodeArchive(raw []byte, accountKey, file string) (*Archive, error)
	DecodeScope(raw []byte, accountKey string, read func(string) ([]byte, error)) (*Scope, error)
}
