package pkg

import "github.com/shanjunmei/dig"

// AuthManager and AppConfig are exported dependencies provided by this module.
type AuthManager struct {
	Token string
}

type AppConfig struct {
	Region string
}

// NewAuthManager / NewAppConfig are exported factories (valid cross-package refs).
func NewAuthManager() *AuthManager { return &AuthManager{Token: "t"} }
func NewAppConfig() *AppConfig     { return &AppConfig{Region: "cn"} }

// buildAuditAuthorizer is an UNEXPORTED factory used inside a closure.
// When digen lifts that closure into the parent (main) package, it must
// reference pkg.buildAuditAuthorizer — which is illegal because the
// function is unexported. digen should detect this before generating.
func buildAuditAuthorizer(authMgr *AuthManager, cfg *AppConfig) *AuthManager {
	return authMgr
}

// Module embeds a closure whose body calls the unexported function above.
func Module() dig.Option {
	return dig.Module(
		dig.Provide(NewAuthManager),
		dig.Provide(NewAppConfig),
		dig.Invoke(func(authMgr *AuthManager, cfg *AppConfig) {
			_ = buildAuditAuthorizer(authMgr, cfg)
		}),
	)
}
