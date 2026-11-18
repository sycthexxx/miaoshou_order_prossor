package packing

// ShippingLabel is the platform's outgoing label, separate from purchase Waybills.
type ShippingLabel struct {
	State   string `json:"state"`
	Number  string `json:"number,omitempty"`
	Message string `json:"message,omitempty"`
	Archive string `json:"archive,omitempty"`
}
