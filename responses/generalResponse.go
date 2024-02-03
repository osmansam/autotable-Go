package responses

type GeneralResponse struct {
    Status  int         `json:"status"`
    Message string      `json:"message"`
    Data    interface{} `json:"data"`
    Source  *string     `json:"source,omitempty"`
}
