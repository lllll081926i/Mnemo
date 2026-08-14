package drive

import (
	"context"

	"mnemo-go/internal/model"
)

// AuthRequest carries inputs for a provider login flow.
type AuthRequest struct {
	// Config holds the login form values keyed by provider (token/username/
	// password/endpoint/bucket/code ...). Exact keys are provider-defined.
	Config map[string]string
	// Open opens a browser/authorization URL (wired by the app layer).
	Open func(url string) error
}

// AuthFunc performs a full provider login and returns the session token.
// For OAuth providers it runs a short-lived localhost callback listener.
type AuthFunc func(ctx context.Context, req AuthRequest) (*model.TokenInfo, error)

// LoginConfig declares the UI form fields needed to log into a provider.
type LoginConfig struct {
	// Fields is an ordered list of form inputs.
	Fields []LoginField `json:"fields"`
}

// LoginField is one login form input.
type LoginField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"` // text | password | token | endpoint | bucket | region | code
	Required bool   `json:"required"`
	Hint     string `json:"hint,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// Registration holds the login fields rendered by the login panel.
func (r Registration) LoginConfig() LoginConfig { return r.Login }