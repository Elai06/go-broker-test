package cmd

type Trade struct {
	ID      int     `json:"-"`
	Account string  `json:"account" validate:"required"`
	Symbol  string  `json:"symbol" validate:"matches=^[A-Z]{6}$"`
	Volume  float64 `json:"volume" validate:"gt=0"`
	Open    float64 `json:"open" validate:"gt=0"`
	Close   float64 `json:"close" validate:"gt=0"`
	Side    string  `json:"side" validate:"oneof=buy sell"`
}
