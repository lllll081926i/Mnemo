package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"mnemo-go/internal/captcha"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/providers/thunder"
)

func (a *App) SendThunderSMS(config map[string]string) (map[string]string, error) {
	requestConfig := make(map[string]string, len(config)+1)
	for key, value := range config {
		requestConfig[key] = value
	}
	session, err := a.startThunderVerification()
	if err != nil {
		return nil, err
	}
	requestConfig["captcha_redirect_uri"] = captcha.RedirectURI(*session)
	result, err := thunder.SendSMS(context.Background(), drive.AuthRequest{Config: requestConfig})
	var challenge *thunder.VerificationRequiredError
	if errors.As(err, &challenge) {
		return nil, fmt.Errorf("%w\nsession=%s", err, session.ID)
	}
	captcha.CloseSession(session.ID)
	return result, err
}

func (a *App) ShowThunderVerification(sessionID, rawURL string) (bool, error) {
	if _, err := thunder.VerificationURL(rawURL); err != nil {
		return false, err
	}
	return captcha.ShowForProvider("thunder", sessionID, rawURL, filepath.Join(a.dataDirectory(), "thunder-verification"))
}

func (a *App) CancelThunderLogin(attemptID, sessionID string) {
	thunder.CancelLogin(attemptID)
	captcha.CloseSession(sessionID)
}

func (a *App) startThunderVerification() (*captcha.Session, error) {
	return captcha.StartForProviderWithResult("thunder", func(session captcha.Session, result captcha.VerificationResult) {
		a.emit("thunder:verification:completed", map[string]string{"session_id": session.ID, "captcha_token": result.CaptchaToken, "creditkey": result.CreditKey})
	})
}
