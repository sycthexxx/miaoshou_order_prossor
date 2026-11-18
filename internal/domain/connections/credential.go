// Package connections describes saved credentials independently of a vendor SDK.
package connections

type Credential struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	APIID        string `json:"apiId"`
	Key          string `json:"-"`
	SubAccountID string `json:"subAccountId"`
	Current      bool   `json:"current"`
}
type Input struct {
	Name         string `json:"name"`
	APIID        string `json:"apiId"`
	Key          string `json:"key"`
	SubAccountID string `json:"subAccountId"`
	Current      bool   `json:"current"`
}
