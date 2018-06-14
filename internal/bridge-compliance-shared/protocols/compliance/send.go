package compliance

import (
	"encoding/json"
	"strings"

	"github.com/stellar/go/protocols/compliance"
	"github.com/stellar/go/services/internal/bridge-compliance-shared/http/helpers"
	"github.com/stellar/go/services/internal/bridge-compliance-shared/protocols"
)

// SendRequest represents request sent to /send endpoint of compliance server
type SendRequest struct {
	// Source account ID
	Source string `name:"source" required:""`
	// Sender address (like alice*stellar.org)
	Sender string `name:"sender"`
	// Destination address (like bob*stellar.org)
	Destination string `name:"destination"`
	// ForwardDestination
	ForwardDestination *protocols.ForwardDestination `name:"forward_destination"`
	// Amount destination should receive
	Amount string `name:"amount" required:""`
	// Code of the asset destination should receive
	AssetCode string `name:"asset_code" required:""`
	// Issuer of the asset destination should receive
	AssetIssuer string `name:"asset_issuer" required:""`
	// Only for path_payment
	SendMax string `name:"send_max"`
	// Only for path_payment
	SendAssetCode string `name:"send_asset_code"`
	// Only for path_payment
	SendAssetIssuer string `name:"send_asset_issuer"`
	// path[n][asset_code] path[n][asset_issuer]
	Path []protocols.Asset `name:"path"`
	// Extra memo
	ExtraMemo string `name:"extra_memo"`
}

// Validate validates if request fields are valid. Useful when checking if a request is correct.
func (request *SendRequest) Validate() error {
	panic("TODO")
	// err := request.FormRequest.CheckRequired(request)
	// if err != nil {
	// 	return err
	// }

	// if !protocols.IsValidAccountID(request.Source) {
	// 	return protocols.NewInvalidParameterError("source", request.Source, "Source must be a public key (starting with `G`).")
	// }

	// if !validateStellarAddress(request.Sender) {
	// 	return protocols.NewInvalidParameterError("sender", request.Sender, "Not a valid stellar address.")
	// }

	// if request.Destination == "" && request.ForwardDestination == nil {
	// 	return protocols.NewMissingParameter("destination")
	// }

	// if request.Destination != "" && !validateStellarAddress(request.Destination) {
	// 	return protocols.NewInvalidParameterError("destination", request.Destination, "Not a valid stellar address.")
	// }

	// _, err = keypair.Parse(request.AssetIssuer)
	// if !protocols.IsValidAccountID(request.AssetIssuer) {
	// 	return protocols.NewInvalidParameterError("asset_issuer", request.AssetIssuer, "Asset issuer must be a public key (starting with `G`).")
	// }

	// return nil
}

func validateStellarAddress(address string) bool {
	tokens := strings.Split(address, "*")
	return len(tokens) == 2
}

// SendResponse represents response returned by /send endpoint
type SendResponse struct {
	helpers.SuccessResponse
	compliance.AuthResponse `json:"auth_response"`
	// xdr.Transaction base64-encoded. Sequence number of this transaction will be equal 0.
	TransactionXdr string `json:"transaction_xdr,omitempty"`
}

// Marshal marshals SendResponse
func (response *SendResponse) Marshal() ([]byte, error) {
	return json.MarshalIndent(response, "", "  ")
}
