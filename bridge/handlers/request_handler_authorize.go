package handlers

import (
	log "github.com/sirupsen/logrus"
	"net/http"

	b "github.com/stellar/go/build"
	"github.com/stellar/go/services/bridge/protocols"
	"github.com/stellar/go/services/bridge/protocols/bridge"
	"github.com/stellar/go/services/bridge/server"
)

// Authorize implements /authorize endpoint
func (rh *RequestHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	request := &bridge.AuthorizeRequest{}
	err := request.FromRequest(r)
	if err != nil {
		log.Error(err.Error())
		server.Write(w, protocols.InvalidParameterError)
		return
	}

	err = request.Validate(rh.Config.Assets, rh.Config.Accounts.IssuingAccountID)
	if err != nil {
		errorResponse := err.(*protocols.ErrorResponse)
		log.WithFields(errorResponse.LogData).Error(errorResponse.Error())
		server.Write(w, errorResponse)
		return
	}

	operationMutator := b.AllowTrust(
		b.Trustor{request.AccountID},
		b.Authorize{true},
		b.AllowTrustAsset{request.AssetCode},
	)

	submitResponse, err := rh.TransactionSubmitter.SubmitTransaction(
		nil,
		rh.Config.Accounts.AuthorizingSeed,
		operationMutator,
		nil,
	)

	if err != nil {
		log.WithFields(log.Fields{"err": err}).Error("Error submitting transaction")
		server.Write(w, protocols.InternalServerError)
		return
	}

	errorResponse := bridge.ErrorFromHorizonResponse(submitResponse)
	if errorResponse != nil {
		log.WithFields(errorResponse.LogData).Error(errorResponse.Error())
		server.Write(w, errorResponse)
		return
	}

	server.Write(w, &submitResponse)
}
