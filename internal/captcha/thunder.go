package captcha

import (
	_ "embed"
	"encoding/json"
)

//go:embed thunder_bootstrap.js
var thunderBootstrap string

func thunderVerificationScript(session Session) string {
	callback, _ := json.Marshal(RedirectURI(session))
	return "(" + thunderBootstrap + ")(" + string(callback) + ");"
}
