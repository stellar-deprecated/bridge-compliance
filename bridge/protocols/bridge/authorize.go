package bridge

import (
	"net/http"
	"net/url"

	"github.com/stellar/go/services/bridge/config"
	"github.com/stellar/go/services/bridge/protocols"
)

var (
	// AllowTrustMalformed is an error response
	AllowTrustMalformed = &protocols.ErrorResponse{Code: "allow_trust_malformed", Message: "Asset name is malformed.", Status: http.StatusBadRequest}
	// AllowTrustNoTrustline is an error response
	AllowTrustNoTrustline = &protocols.ErrorResponse{Code: "allow_trust_no_trustline", Message: "Trustor does not have a trustline yet.", Status: http.StatusBadRequest}
	// AllowTrustTrustNotRequired is an error response
	AllowTrustTrustNotRequired = &protocols.ErrorResponse{Code: "allow_trust_trust_not_required", Message: "Authorizing account does not require allowing trust. Set AUTH_REQUIRED_FLAG on your account to use this feature.", Status: http.StatusBadRequest}
	// AllowTrustCantRevoke is an error response
	AllowTrustCantRevoke = &protocols.ErrorResponse{Code: "allow_trust_cant_revoke", Message: "Authorizing account has AUTH_REVOCABLE_FLAG set. Can't revoke the trustline.", Status: http.StatusBadRequest}
)

// AuthorizeRequest represents request made to /authorize endpoint of bridge server
type AuthorizeRequest struct {
	AccountID string `name:"account_id" required:""`
	AssetCode string `name:"asset_code" required:""`

	protocols.FormRequest
}

// FromRequest will populate request fields using http.Request.
func (request *AuthorizeRequest) FromRequest(r *http.Request) error {
	return request.FormRequest.FromRequest(r, request)
}

// ToValues will create url.Values from request.
func (request *AuthorizeRequest) ToValues() url.Values {
	return request.FormRequest.ToValues(request)
}

// Validate validates if request fields are valid. Useful when checking if a request is correct.
func (request *AuthorizeRequest) Validate(allowedAssets []config.Asset, issuingAccountID string) error {
	err := request.FormRequest.CheckRequired(request)
	if err != nil {
		return err
	}

	if !protocols.IsValidAccountID(request.AccountID) {
		return protocols.NewInvalidParameterError("account_id", request.AccountID, "Account ID must start with `G`.")
	}

	// Is asset allowed?
	allowed := false
	for _, asset := range allowedAssets {
		if asset.Code == request.AssetCode && asset.Issuer == issuingAccountID {
			allowed = true
			break
		}
	}

	if !allowed {
		return protocols.NewInvalidParameterError("asset_code", request.AssetCode, "Asset code not allowed.")
	}

	return nil
}
