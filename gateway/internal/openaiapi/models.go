package openaiapi

type ModelList struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	OwnedBy string    `json:"owned_by"`
	Meta    ModelMeta `json:"meta"`
}

type ModelMeta struct {
	Downloaded   bool   `json:"downloaded"`
	RouterStatus string `json:"router_status,omitempty"`
	Running      bool   `json:"running"`
	ColdStart    bool   `json:"cold_start"`
	Repo         string `json:"repo"`
	Quant        string `json:"quant"`
	Kind         string `json:"kind,omitempty"`
}
