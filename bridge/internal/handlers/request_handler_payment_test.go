package handlers

import (
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stellar/go/build"
	"github.com/stellar/go/protocols/federation"
	"github.com/stellar/go/services/bridge/internal/config"
	"github.com/stellar/go/services/bridge/internal/db/entities"
	"github.com/stellar/go/services/bridge/internal/horizon"
	"github.com/stellar/go/services/bridge/internal/mocks"
	"github.com/stellar/go/services/bridge/internal/net"
	callback "github.com/stellar/go/services/bridge/internal/protocols/compliance"
	"github.com/stellar/go/services/bridge/internal/test"
	"github.com/stellar/go/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRequestHandlerPayment(t *testing.T) {
	c := &config.Config{
		NetworkPassphrase: "Test SDF Network ; September 2015",
		Compliance:        "http://compliance",
		Accounts: config.Accounts{
			// GAHA6GRCLCCN7XE2NEEUDSIVOFBOQ6GLSYXVLYCJXJKLPMDR5XB5XZZJ
			BaseSeed: "SBKKWO3ZVDDEHDJILGHPHCJCFD2GNUAYIUDMRAS326HLUEQ7ZFXWIGQK",
		},
	}
	mockHorizon := new(mocks.MockHorizon)
	mockRepository := new(mocks.MockRepository)
	mockHTTPClient := new(mocks.MockHTTPClient)
	mockTransactionSubmitter := new(mocks.MockTransactionSubmitter)
	mockFederationResolver := new(mocks.MockFederationResolver)
	mockStellartomlResolver := new(mocks.MockStellartomlResolver)

	requestHandler := RequestHandler{
		Config:               c,
		Client:               mockHTTPClient,
		Horizon:              mockHorizon,
		Repository:           mockRepository,
		TransactionSubmitter: mockTransactionSubmitter,
		FederationResolver:   mockFederationResolver,
		StellarTomlResolver:  mockStellartomlResolver,
	}

	testServer := httptest.NewServer(http.HandlerFunc(requestHandler.Payment))
	defer testServer.Close()

	Convey("Given payment request", t, func() {
		Convey("When source is invalid", func() {
			params := url.Values{
				"source":      {"SDRAS7XIQNX25UDCCX725R4EYGBFYGJE4HJ2A3DFCWJIHMRSMS7CXX43"},
				"destination": {"GBABZMS7MEDWKWSHOMUKAWGIOE5UA4XLVPUHRHVMUW2DUVEZXLH5OIET"},
				"amount":      {"20.0"},
			}

			Convey("it should return error", func() {
				statusCode, response := net.GetResponse(testServer, params)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 400, statusCode)
				expected := test.StringToJSONMap(`{
  "code": "invalid_parameter",
  "message": "Invalid parameter.",
  "data": {
    "name": "source"
  }
}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString, "more_info"))
			})
		})

		Convey("When destination is invalid", func() {
			params := url.Values{
				"source":      {"SDRAS7XIQNX25UDCCX725R4EYGBFYGJE4HJ2A3DFCWJIHMRSMS7CXX42"},
				"destination": {"GD3YBOYIUVLU"},
				"amount":      {"20.0"},
			}

			mockFederationResolver.On(
				"LookupByAddress",
				"GD3YBOYIUVLU",
			).Return(
				&federation.NameResponse{AccountID: "GD3YBOYIUVLU"},
				nil,
			).Once()

			Convey("it should return error", func() {
				statusCode, response := net.GetResponse(testServer, params)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 400, statusCode)
				expected := test.StringToJSONMap(`{
  "code": "invalid_parameter",
  "message": "Invalid parameter.",
  "data": {
    "name": "destination"
  }
}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString, "more_info"))
			})
		})

		Convey("When destination is a public key", func() {
			validParams := url.Values{
				// GBKGH7QZVCZ2ZA5OUGZSTHFNXTBHL3MPCKSCBJUAQODGPMWP7OMMRKDW
				"source":      {"SDRAS7XIQNX25UDCCX725R4EYGBFYGJE4HJ2A3DFCWJIHMRSMS7CXX42"},
				"destination": {"GAPCT362RATBUJ37RN2MOKQIZLHSJMO33MMCSRUXTTHIGVDYWOFG5HDS"},
				"amount":      {"20.0"},
			}

			// Checking if destination account exists
			mockHorizon.On(
				"LoadAccount",
				"GAPCT362RATBUJ37RN2MOKQIZLHSJMO33MMCSRUXTTHIGVDYWOFG5HDS",
			).Return(horizon.AccountResponse{}, nil).Once()

			var ledger uint64
			ledger = 1988728
			horizonResponse := horizon.TransactionSuccess{
				Hash:   "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
				Ledger: &ledger,
				Extras: nil,
			}

			mockTransactionSubmitter.On(
				"SubmitTransaction",
				mock.AnythingOfType("*string"),
				"SDRAS7XIQNX25UDCCX725R4EYGBFYGJE4HJ2A3DFCWJIHMRSMS7CXX42",
				mock.AnythingOfType("build.PaymentBuilder"),
				nil,
			).Run(func(args mock.Arguments) {
				operation, ok := args.Get(2).(build.PaymentBuilder)
				assert.True(t, ok, "Invalid conversion")
				assert.Equal(t, "GAPCT362RATBUJ37RN2MOKQIZLHSJMO33MMCSRUXTTHIGVDYWOFG5HDS", operation.P.Destination.Address())
				assert.Equal(t, int64(200000000), int64(operation.P.Amount))
				assert.Equal(t, xdr.AssetTypeAssetTypeNative, operation.P.Asset.Type)
			}).Return(horizonResponse, nil).Once()

			Convey("it should return success", func() {
				statusCode, response := net.GetResponse(testServer, validParams)
				responseString := strings.TrimSpace(string(response))

				assert.Equal(t, 200, statusCode)
				expected := test.StringToJSONMap(`{
					  "hash": "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
					  "ledger": 1988728
					}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString))
			})
		})

		Convey("When destination is a Stellar address", func() {
			params := url.Values{
				"source":      {"SDRAS7XIQNX25UDCCX725R4EYGBFYGJE4HJ2A3DFCWJIHMRSMS7CXX42"},
				"destination": {"bob*stellar.org"},
				"amount":      {"20.0"},
			}

			Convey("When FederationResolver returns error", func() {
				mockFederationResolver.On(
					"LookupByAddress",
					"bob*stellar.org",
				).Return(
					&federation.NameResponse{},
					errors.New("stellar.toml response status code indicates error"),
				).Once()

				Convey("it should return error", func() {
					statusCode, response := net.GetResponse(testServer, params)
					responseString := strings.TrimSpace(string(response))
					assert.Equal(t, 400, statusCode)
					expected := test.StringToJSONMap(`{
  "code": "cannot_resolve_destination",
  "message": "Cannot resolve federated Stellar address."
}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})

			Convey("When federation response is correct (no memo)", func() {
				validParams := url.Values{
					// GCF3WVYTHF75PEG6622G5G6KU26GOSDQPDHSCJ3DQD7VONH4EYVDOGKJ
					"source":      {"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM"},
					"destination": {"bob*stellar.org"},
					"amount":      {"20"},
				}

				mockFederationResolver.On(
					"LookupByAddress",
					"bob*stellar.org",
				).Return(
					&federation.NameResponse{AccountID: "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
					nil,
				).Once()

				// Checking if destination account exists
				mockHorizon.On(
					"LoadAccount",
					"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
				).Return(horizon.AccountResponse{}, nil).Once()

				var ledger uint64
				ledger = 1988728
				horizonResponse := horizon.TransactionSuccess{
					Hash:   "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
					Ledger: &ledger,
					Extras: nil,
				}

				mockTransactionSubmitter.On(
					"SubmitTransaction",
					mock.AnythingOfType("*string"),
					"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
					mock.AnythingOfType("build.PaymentBuilder"),
					nil,
				).Run(func(args mock.Arguments) {
					operation, ok := args.Get(2).(build.PaymentBuilder)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.P.Destination.Address())
					assert.Equal(t, int64(200000000), int64(operation.P.Amount))
					assert.Equal(t, xdr.AssetTypeAssetTypeNative, operation.P.Asset.Type)
				}).Return(horizonResponse, nil).Once()

				Convey("it should return success", func() {
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
					  "ledger": 1988728
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})

			Convey("When federation response is correct (with memo)", func() {
				validParams := url.Values{
					// GCF3WVYTHF75PEG6622G5G6KU26GOSDQPDHSCJ3DQD7VONH4EYVDOGKJ
					"source":      {"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM"},
					"destination": {"bob*stellar.org"},
					"amount":      {"20"},
				}

				mockFederationResolver.On(
					"LookupByAddress",
					"bob*stellar.org",
				).Return(
					&federation.NameResponse{
						AccountID: "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
						MemoType:  "text",
						Memo:      federation.Memo{"125"},
					},
					nil,
				).Once()

				// Checking if destination account exists
				mockHorizon.On(
					"LoadAccount",
					"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
				).Return(horizon.AccountResponse{}, nil).Once()

				var ledger uint64
				ledger = 1988728
				horizonResponse := horizon.TransactionSuccess{
					Hash:   "ad71fc31bfae25b0bd14add4cc5306661edf84cdd73f1353d2906363899167e1",
					Ledger: &ledger,
					Extras: nil,
				}

				mockTransactionSubmitter.On(
					"SubmitTransaction",
					mock.AnythingOfType("*string"),
					"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
					mock.AnythingOfType("build.PaymentBuilder"),
					mock.AnythingOfType("build.MemoText"),
				).Run(func(args mock.Arguments) {
					operation, ok := args.Get(2).(build.PaymentBuilder)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.P.Destination.Address())
					assert.Equal(t, int64(200000000), int64(operation.P.Amount))
					assert.Equal(t, xdr.AssetTypeAssetTypeNative, operation.P.Asset.Type)

					memo, ok := args.Get(3).(build.MemoText)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, "125", memo.Value)
				}).Return(horizonResponse, nil).Once()

				Convey("it should return success", func() {
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "ad71fc31bfae25b0bd14add4cc5306661edf84cdd73f1353d2906363899167e1",
					  "ledger": 1988728
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})
		})

		Convey("When forward federation destination", func() {
			params := url.Values{
				"source":                                       {"SDRAS7XIQNX25UDCCX725R4EYGBFYGJE4HJ2A3DFCWJIHMRSMS7CXX42"},
				"forward_destination[domain]":                  {"stellar.org"},
				"forward_destination[fields][federation_type]": {"bank_account"},
				"forward_destination[fields][swift]":           {"BOPBPHMM"},
				"forward_destination[fields][acct]":            {"2382376"},
				"amount": {"20.0"},
			}

			Convey("When FederationResolver returns error", func() {
				mockFederationResolver.On(
					"ForwardRequest",
					"stellar.org",
					url.Values{
						"federation_type": {"bank_account"},
						"swift":           {"BOPBPHMM"},
						"acct":            {"2382376"},
					},
				).Return(
					&federation.NameResponse{},
					errors.New("stellar.toml response status code indicates error"),
				).Once()

				Convey("it should return error", func() {
					statusCode, response := net.GetResponse(testServer, params)
					responseString := strings.TrimSpace(string(response))
					assert.Equal(t, 400, statusCode)
					expected := test.StringToJSONMap(`{
  "code": "cannot_resolve_destination",
  "message": "Cannot resolve federated Stellar address."
}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})

			Convey("When federation response is correct (no memo)", func() {
				validParams := url.Values{
					// GCF3WVYTHF75PEG6622G5G6KU26GOSDQPDHSCJ3DQD7VONH4EYVDOGKJ
					"source":                                       {"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM"},
					"forward_destination[domain]":                  {"stellar.org"},
					"forward_destination[fields][federation_type]": {"bank_account"},
					"forward_destination[fields][swift]":           {"BOPBPHMM"},
					"forward_destination[fields][acct]":            {"2382376"},
					"amount": {"20"},
				}

				mockFederationResolver.On(
					"ForwardRequest",
					"stellar.org",
					url.Values{
						"federation_type": {"bank_account"},
						"swift":           {"BOPBPHMM"},
						"acct":            {"2382376"},
					},
				).Return(
					&federation.NameResponse{AccountID: "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
					nil,
				).Once()

				// Checking if destination account exists
				mockHorizon.On(
					"LoadAccount",
					"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
				).Return(horizon.AccountResponse{}, nil).Once()

				var ledger uint64
				ledger = 1988728
				horizonResponse := horizon.TransactionSuccess{
					Hash:   "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
					Ledger: &ledger,
					Extras: nil,
				}

				mockTransactionSubmitter.On(
					"SubmitTransaction",
					mock.AnythingOfType("*string"),
					"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
					mock.AnythingOfType("build.PaymentBuilder"),
					nil,
				).Run(func(args mock.Arguments) {
					operation, ok := args.Get(2).(build.PaymentBuilder)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.P.Destination.Address())
					assert.Equal(t, int64(200000000), int64(operation.P.Amount))
					assert.Equal(t, xdr.AssetTypeAssetTypeNative, operation.P.Asset.Type)
				}).Return(horizonResponse, nil).Once()

				Convey("it should return success", func() {
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
					  "ledger": 1988728
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})

			Convey("When federation response is correct (with memo)", func() {
				validParams := url.Values{
					// GCF3WVYTHF75PEG6622G5G6KU26GOSDQPDHSCJ3DQD7VONH4EYVDOGKJ
					"source":                                       {"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM"},
					"forward_destination[domain]":                  {"stellar.org"},
					"forward_destination[fields][federation_type]": {"bank_account"},
					"forward_destination[fields][swift]":           {"BOPBPHMM"},
					"forward_destination[fields][acct]":            {"2382376"},
					"amount": {"20"},
				}

				mockFederationResolver.On(
					"ForwardRequest",
					"stellar.org",
					url.Values{
						"federation_type": {"bank_account"},
						"swift":           {"BOPBPHMM"},
						"acct":            {"2382376"},
					},
				).Return(
					&federation.NameResponse{
						AccountID: "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
						MemoType:  "text",
						Memo:      federation.Memo{"125"},
					},
					nil,
				).Once()

				// Checking if destination account exists
				mockHorizon.On(
					"LoadAccount",
					"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
				).Return(horizon.AccountResponse{}, nil).Once()

				var ledger uint64
				ledger = 1988728
				horizonResponse := horizon.TransactionSuccess{
					Hash:   "ad71fc31bfae25b0bd14add4cc5306661edf84cdd73f1353d2906363899167e1",
					Ledger: &ledger,
					Extras: nil,
				}

				mockTransactionSubmitter.On(
					"SubmitTransaction",
					mock.AnythingOfType("*string"),
					"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
					mock.AnythingOfType("build.PaymentBuilder"),
					mock.AnythingOfType("build.MemoText"),
				).Run(func(args mock.Arguments) {
					operation, ok := args.Get(2).(build.PaymentBuilder)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.P.Destination.Address())
					assert.Equal(t, int64(200000000), int64(operation.P.Amount))
					assert.Equal(t, xdr.AssetTypeAssetTypeNative, operation.P.Asset.Type)

					memo, ok := args.Get(3).(build.MemoText)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, "125", memo.Value)
				}).Return(horizonResponse, nil).Once()

				Convey("it should return success", func() {
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "ad71fc31bfae25b0bd14add4cc5306661edf84cdd73f1353d2906363899167e1",
					  "ledger": 1988728
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})
		})

		Convey("When asset_issuer is invalid", func() {
			params := url.Values{
				"source":       {"SDRAS7XIQNX25UDCCX725R4EYGBFYGJE4HJ2A3DFCWJIHMRSMS7CXX42"},
				"destination":  {"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				"asset_code":   {"USD"},
				"asset_issuer": {"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN631"},
				"amount":       {"100.0"},
			}

			mockFederationResolver.On(
				"LookupByAddress",
				"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
			).Return(
				&federation.NameResponse{AccountID: "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				nil,
			).Once()

			Convey("it should return error", func() {
				statusCode, response := net.GetResponse(testServer, params)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 400, statusCode)
				expected := test.StringToJSONMap(`{
  "code": "invalid_parameter",
  "message": "Invalid parameter.",
  "data": {
    "name": "asset"
  }
}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString, "more_info"))
			})
		})

		Convey("When assetCode is invalid", func() {
			// GBKGH7QZVCZ2ZA5OUGZSTHFNXTBHL3MPCKSCBJUAQODGPMWP7OMMRKDW
			source := "SDRAS7XIQNX25UDCCX725R4EYGBFYGJE4HJ2A3DFCWJIHMRSMS7CXX42"
			destination := "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"
			amount := "20"
			assetCode := "1234567890123"
			assetIssuer := "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"

			mockFederationResolver.On(
				"LookupByAddress",
				"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
			).Return(
				&federation.NameResponse{AccountID: "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				nil,
			).Once()

			Convey("it should return error", func() {
				mockHorizon.On(
					"LoadAccount",
					"GBKGH7QZVCZ2ZA5OUGZSTHFNXTBHL3MPCKSCBJUAQODGPMWP7OMMRKDW",
				).Return(
					horizon.AccountResponse{
						SequenceNumber: "100",
					},
					nil,
				).Once()

				statusCode, response := net.GetResponse(
					testServer,
					url.Values{
						"source":       {source},
						"destination":  {destination},
						"amount":       {amount},
						"asset_code":   {assetCode},
						"asset_issuer": {assetIssuer},
					},
				)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 400, statusCode)
				expected := test.StringToJSONMap(`{
  "code": "invalid_parameter",
  "message": "Invalid parameter.",
  "data": {
    "name": "asset"
  }
}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString, "more_info"))
			})
		})

		Convey("When amount is invalid", func() {
			// GBKGH7QZVCZ2ZA5OUGZSTHFNXTBHL3MPCKSCBJUAQODGPMWP7OMMRKDW
			source := "SDRAS7XIQNX25UDCCX725R4EYGBFYGJE4HJ2A3DFCWJIHMRSMS7CXX42"
			destination := "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"
			amount := "test"
			assetCode := "USD"
			assetIssuer := "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"

			mockFederationResolver.On(
				"LookupByAddress",
				"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
			).Return(
				&federation.NameResponse{AccountID: "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				nil,
			).Once()

			mockHorizon.On(
				"LoadAccount",
				"GBKGH7QZVCZ2ZA5OUGZSTHFNXTBHL3MPCKSCBJUAQODGPMWP7OMMRKDW",
			).Return(
				horizon.AccountResponse{
					SequenceNumber: "100",
				},
				nil,
			).Once()

			Convey("it should return error", func() {
				statusCode, response := net.GetResponse(
					testServer,
					url.Values{
						"source":       {source},
						"destination":  {destination},
						"amount":       {amount},
						"asset_code":   {assetCode},
						"asset_issuer": {assetIssuer},
					},
				)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 400, statusCode)
				expected := test.StringToJSONMap(`{
  "code": "invalid_parameter",
  "message": "Invalid parameter.",
  "data": {
    "name": "amount"
  }
}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString, "more_info"))
			})
		})

		Convey("When params are valid - base account (no source param)", func() {
			validParams := url.Values{
				"destination":  {"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				"amount":       {"20"},
				"asset_code":   {"USD"},
				"asset_issuer": {"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
			}

			// Federation response
			mockFederationResolver.On(
				"LookupByAddress",
				"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
			).Return(
				&federation.NameResponse{AccountID: "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				nil,
			).Once()

			var ledger uint64
			ledger = 1988728
			horizonResponse := horizon.TransactionSuccess{
				Hash:   "ad71fc31bfae25b0bd14add4cc5306661edf84cdd73f1353d2906363899167e1",
				Ledger: &ledger,
				Extras: nil,
			}

			mockTransactionSubmitter.On(
				"SubmitTransaction",
				mock.AnythingOfType("*string"),
				"SBKKWO3ZVDDEHDJILGHPHCJCFD2GNUAYIUDMRAS326HLUEQ7ZFXWIGQK",
				mock.AnythingOfType("build.PaymentBuilder"),
				nil,
			).Run(func(args mock.Arguments) {
				operation, ok := args.Get(2).(build.PaymentBuilder)
				assert.True(t, ok, "Invalid conversion")
				assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.P.Destination.Address())
				assert.Equal(t, int64(200000000), int64(operation.P.Amount))
				assert.Equal(t, xdr.AssetTypeAssetTypeCreditAlphanum4, operation.P.Asset.Type)
				// assert.Equal(t, "USD", operation.P.Asset.Code)
			}).Return(horizonResponse, nil).Once()

			Convey("it should return success", func() {
				statusCode, response := net.GetResponse(testServer, validParams)
				responseString := strings.TrimSpace(string(response))

				assert.Equal(t, 200, statusCode)
				expected := test.StringToJSONMap(`{
				  "hash": "ad71fc31bfae25b0bd14add4cc5306661edf84cdd73f1353d2906363899167e1",
				  "ledger": 1988728
				}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString))
			})
		})

		Convey("When params are valid (payment operation)", func() {
			validParams := url.Values{
				// GCF3WVYTHF75PEG6622G5G6KU26GOSDQPDHSCJ3DQD7VONH4EYVDOGKJ
				"source":       {"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM"},
				"destination":  {"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				"amount":       {"20"},
				"asset_code":   {"USD"},
				"asset_issuer": {"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
			}

			mockFederationResolver.On(
				"LookupByAddress",
				"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
			).Return(
				&federation.NameResponse{AccountID: "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				nil,
			).Once()

			Convey("When memo is set", func() {
				Convey("only `memo` param is set", func() {
					validParams.Add("memo", "test")
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))
					assert.Equal(t, 400, statusCode)
					expected := test.StringToJSONMap(`{
  "code": "missing_parameter",
  "message": "Required parameter is missing.",
  "data": {
    "name": "memo_type"
  }
}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})

				Convey("only `memo_type` param is set", func() {
					validParams.Add("memo_type", "id")
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))
					assert.Equal(t, 400, statusCode)
					expected := test.StringToJSONMap(`{
  "code": "missing_parameter",
  "message": "Required parameter is missing.",
  "data": {
    "name": "memo"
  }
}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})

				Convey("memo_type=hash to long", func() {
					validParams.Add("memo_type", "hash")
					validParams.Add("memo", "012345678901234567890123456789012345678901234567890123456789012")
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))
					assert.Equal(t, 400, statusCode)
					expected := test.StringToJSONMap(`{
  "code": "invalid_parameter",
  "message": "Invalid parameter.",
  "data": {
    "name": "memo"
  }
}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString, "more_info"))
				})

				Convey("unsupported memo_type", func() {
					validParams.Add("memo_type", "return_hash")
					validParams.Add("memo", "0123456789")
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))
					assert.Equal(t, 400, statusCode)
					expected := test.StringToJSONMap(`{
  "code": "invalid_parameter",
  "message": "Invalid parameter.",
  "data": {
    "name": "memo"
  }
}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString, "more_info"))
				})

				Convey("memo is attached to the transaction", func() {
					var ledger uint64
					ledger = 1988727
					horizonResponse := horizon.TransactionSuccess{
						Hash:   "f16040c1c6ee29eb4cc6f797651901750ff48a203985eea74f94353502f6629d",
						Ledger: &ledger,
						Extras: nil,
					}

					mockTransactionSubmitter.On(
						"SubmitTransaction",
						mock.AnythingOfType("*string"),
						"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
						mock.AnythingOfType("build.PaymentBuilder"),
						mock.AnythingOfType("build.MemoID"),
					).Run(func(args mock.Arguments) {
						operation, ok := args.Get(2).(build.PaymentBuilder)
						assert.True(t, ok, "Invalid conversion")
						assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.P.Destination.Address())
						assert.Equal(t, int64(200000000), int64(operation.P.Amount))
						assert.Equal(t, xdr.AssetTypeAssetTypeCreditAlphanum4, operation.P.Asset.Type)
						// assert.Equal(t, "USD", operation.P.Asset.Code)

						memo, ok := args.Get(3).(build.MemoID)
						assert.True(t, ok, "Invalid conversion")
						assert.Equal(t, uint64(123), memo.Value)
					}).Return(horizonResponse, nil).Once()

					validParams.Add("memo_type", "id")
					validParams.Add("memo", "123")
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "f16040c1c6ee29eb4cc6f797651901750ff48a203985eea74f94353502f6629d",
					  "ledger": 1988727
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})

				Convey("memo hash is attached to the transaction", func() {
					mockHorizon.On(
						"LoadAccount",
						"GCF3WVYTHF75PEG6622G5G6KU26GOSDQPDHSCJ3DQD7VONH4EYVDOGKJ",
					).Return(
						horizon.AccountResponse{
							SequenceNumber: "100",
						},
						nil,
					).Once()

					var ledger uint64
					ledger = 1988727
					horizonResponse := horizon.TransactionSuccess{
						Hash:   "b6802ab06786c923d7180236a84470c03b37ec71912bfe335d0cb57ebc534881",
						Ledger: &ledger,
						Extras: nil,
					}

					mockTransactionSubmitter.On(
						"SubmitTransaction",
						mock.AnythingOfType("*string"),
						"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
						mock.AnythingOfType("build.PaymentBuilder"),
						mock.AnythingOfType("build.MemoHash"),
					).Run(func(args mock.Arguments) {
						operation, ok := args.Get(2).(build.PaymentBuilder)
						assert.True(t, ok, "Invalid conversion")
						assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.P.Destination.Address())
						assert.Equal(t, int64(200000000), int64(operation.P.Amount))
						assert.Equal(t, xdr.AssetTypeAssetTypeCreditAlphanum4, operation.P.Asset.Type)
						// assert.Equal(t, "USD", operation.P.Asset.Code)

						memo, ok := args.Get(3).(build.MemoHash)
						assert.True(t, ok, "Invalid conversion")
						assert.Equal(t, "02003ad420744cdeb8e524deb65f38cb5095d30d000000000000000000000000", hex.EncodeToString([]byte(memo.Value[:])))
					}).Return(horizonResponse, nil).Once()

					validParams.Add("memo_type", "hash")
					validParams.Add("memo", "02003ad420744cdeb8e524deb65f38cb5095d30d000000000000000000000000")
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "b6802ab06786c923d7180236a84470c03b37ec71912bfe335d0cb57ebc534881",
					  "ledger": 1988727
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})

			Convey("transaction failed in horizon", func() {
				horizonResponse := horizon.TransactionSuccess{
					Ledger: nil,
					Extras: &horizon.TransactionSuccessExtras{
						EnvelopeXdr: "envelope",
						ResultXdr:   "AAAAAAAAAAD////7AAAAAA==", // tx_bad_seq
					},
				}

				mockTransactionSubmitter.On(
					"SubmitTransaction",
					mock.AnythingOfType("*string"),
					mock.AnythingOfType("string"),
					mock.AnythingOfType("build.PaymentBuilder"),
					nil,
				).Return(horizonResponse, nil).Once()

				Convey("it should return error", func() {
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 400, statusCode)
					expected := test.StringToJSONMap(`{
  "code": "transaction_bad_seq",
  "message": "Bad Sequence. Please, try again."
}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})

			Convey("transaction success (native)", func() {
				validParams := url.Values{
					// GCF3WVYTHF75PEG6622G5G6KU26GOSDQPDHSCJ3DQD7VONH4EYVDOGKJ
					"source":      {"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM"},
					"destination": {"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
					"amount":      {"20"},
				}

				// Checking if destination exists
				mockHorizon.On(
					"LoadAccount",
					"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
				).Return(horizon.AccountResponse{}, nil).Once()

				var ledger uint64
				ledger = 1988727
				horizonResponse := horizon.TransactionSuccess{
					Hash:   "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
					Ledger: &ledger,
					Extras: nil,
				}

				mockTransactionSubmitter.On(
					"SubmitTransaction",
					mock.AnythingOfType("*string"),
					"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
					mock.AnythingOfType("build.PaymentBuilder"),
					nil,
				).Run(func(args mock.Arguments) {
					operation, ok := args.Get(2).(build.PaymentBuilder)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.P.Destination.Address())
					assert.Equal(t, int64(200000000), int64(operation.P.Amount))
					assert.Equal(t, xdr.AssetTypeAssetTypeNative, operation.P.Asset.Type)
				}).Return(horizonResponse, nil).Once()

				Convey("it should return success", func() {
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
					  "ledger": 1988727
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})
		})

		Convey("When params are valid (path payment operation)", func() {
			validParams := url.Values{
				// GCF3WVYTHF75PEG6622G5G6KU26GOSDQPDHSCJ3DQD7VONH4EYVDOGKJ
				"source":       {"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM"},
				"destination":  {"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				"amount":       {"20"},
				"asset_code":   {"USD"},
				"asset_issuer": {"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				"send_max":     {"100"},
			}

			// Destination
			mockFederationResolver.On(
				"LookupByAddress",
				"GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632",
			).Return(
				&federation.NameResponse{AccountID: "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632"},
				nil,
			).Once()

			Convey("transaction success (send native)", func() {
				var ledger uint64
				ledger = 1988727
				horizonResponse := horizon.TransactionSuccess{
					Hash:   "88214f536658717d5a7d96e449d2fbd96277ce16f3d88dea023e5f20bd37325d",
					Ledger: &ledger,
					Extras: nil,
				}

				mockTransactionSubmitter.On(
					"SubmitTransaction",
					mock.AnythingOfType("*string"),
					"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
					mock.AnythingOfType("build.PaymentBuilder"),
					nil,
				).Run(func(args mock.Arguments) {
					paymentID, ok := args.Get(0).(*string)
					assert.True(t, ok, "Invalid conversion")
					assert.Nil(t, paymentID)

					operation, ok := args.Get(2).(build.PaymentBuilder)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.PP.Destination.Address())
					assert.Equal(t, int64(1000000000), int64(operation.PP.SendMax))
					assert.Equal(t, int64(200000000), int64(operation.PP.DestAmount))
					assert.Equal(t, xdr.AssetTypeAssetTypeNative, operation.PP.SendAsset.Type)
					assert.Equal(t, xdr.AssetTypeAssetTypeCreditAlphanum4, operation.PP.DestAsset.Type)
				}).Return(horizonResponse, nil).Once()

				Convey("it should return success", func() {
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "88214f536658717d5a7d96e449d2fbd96277ce16f3d88dea023e5f20bd37325d",
					  "ledger": 1988727
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})

			Convey("transaction success (id set)", func() {
				validParams["id"] = []string{"payment123"}

				var ledger uint64
				ledger = 1988727
				horizonResponse := horizon.TransactionSuccess{
					Hash:   "88214f536658717d5a7d96e449d2fbd96277ce16f3d88dea023e5f20bd37325d",
					Ledger: &ledger,
					Extras: nil,
				}

				Convey("id not used", func() {
					mockRepository.On(
						"GetSentTransactionByPaymentID",
						validParams["id"][0],
					).Return(nil, nil).Once()

					mockTransactionSubmitter.On(
						"SubmitTransaction",
						mock.AnythingOfType("*string"),
						"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
						mock.AnythingOfType("build.PaymentBuilder"),
						nil,
					).Run(func(args mock.Arguments) {
						paymentID, ok := args.Get(0).(*string)
						assert.True(t, ok, "Invalid conversion")
						require.NotNil(t, paymentID)
						assert.Equal(t, validParams["id"][0], *paymentID)

						operation, ok := args.Get(2).(build.PaymentBuilder)
						require.True(t, ok, "Invalid conversion")
						assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.PP.Destination.Address())
						assert.Equal(t, int64(1000000000), int64(operation.PP.SendMax))
						assert.Equal(t, int64(200000000), int64(operation.PP.DestAmount))
						assert.Equal(t, xdr.AssetTypeAssetTypeNative, operation.PP.SendAsset.Type)
						assert.Equal(t, xdr.AssetTypeAssetTypeCreditAlphanum4, operation.PP.DestAsset.Type)
					}).Return(horizonResponse, nil).Once()

					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "88214f536658717d5a7d96e449d2fbd96277ce16f3d88dea023e5f20bd37325d",
					  "ledger": 1988727
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})

				Convey("id already used", func() {
					sentTransaction := &entities.SentTransaction{
						PaymentID:   &validParams["id"][0],
						EnvelopeXdr: "envelope_xdr",
					}

					mockRepository.On(
						"GetSentTransactionByPaymentID",
						validParams["id"][0],
					).Return(sentTransaction, nil).Once()

					mockHorizon.
						On("SubmitTransaction", sentTransaction.EnvelopeXdr).
						Return(horizonResponse, nil).Once()

					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "88214f536658717d5a7d96e449d2fbd96277ce16f3d88dea023e5f20bd37325d",
					  "ledger": 1988727
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})

			Convey("transaction success (send credit)", func() {
				validParams["send_asset_code"] = []string{"USD"}
				validParams["send_asset_issuer"] = []string{"GBDOSO3K4JTGSWJSIHXAOFIBMAABVM3YK3FI6VJPKIHHM56XAFIUCGD6"}

				var ledger uint64
				ledger = 1988727
				horizonResponse := horizon.TransactionSuccess{
					Hash:   "8d143f846c2e0ce20364be737c2ebdbcd0da307b4952ec8e91ffcbbc6f51f5ce",
					Ledger: &ledger,
					Extras: nil,
				}

				mockTransactionSubmitter.On(
					"SubmitTransaction",
					mock.AnythingOfType("*string"),
					"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
					mock.AnythingOfType("build.PaymentBuilder"),
					nil,
				).Run(func(args mock.Arguments) {
					operation, ok := args.Get(2).(build.PaymentBuilder)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.PP.Destination.Address())
					assert.Equal(t, int64(1000000000), int64(operation.PP.SendMax))
					assert.Equal(t, int64(200000000), int64(operation.PP.DestAmount))
					assert.Equal(t, xdr.AssetTypeAssetTypeCreditAlphanum4, operation.PP.SendAsset.Type)
					assert.Equal(t, xdr.AssetTypeAssetTypeCreditAlphanum4, operation.PP.DestAsset.Type)
				}).Return(horizonResponse, nil).Once()

				Convey("it should return success", func() {
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "8d143f846c2e0ce20364be737c2ebdbcd0da307b4952ec8e91ffcbbc6f51f5ce",
					  "ledger": 1988727
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})

			Convey("transaction success (path)", func() {
				validParams["send_asset_code"] = []string{"USD"}
				validParams["send_asset_issuer"] = []string{"GBDOSO3K4JTGSWJSIHXAOFIBMAABVM3YK3FI6VJPKIHHM56XAFIUCGD6"}

				// Native
				validParams["path[0][asset_code]"] = []string{""}
				validParams["path[0][asset_issuer]"] = []string{""}
				// Credit
				validParams["path[1][asset_code]"] = []string{"EUR"}
				validParams["path[1][asset_issuer]"] = []string{"GAF3PBFQLH57KPECN4GRGHU5NUZ3XXKYYWLOTBIRJMBYHPUBWANIUCZU"}

				var ledger uint64
				ledger = 1988727
				resultXdr := "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAACAAAAAAAAAAEAAAAAC8RjSvPMPWeQWzLq8JEM0BQNo0TfJQN/RwkCeJ+rT+YAAAAAAAAAAwAAAAFaQVIAAAAAAGDBYXf7bGrEkzodp+6aowtAynuEqzKzZRZKO2ftxMtDAAAAAa9EDYAAAAABVVNEAAAAAABstavC6cvn5h86pWOK5996Ape9k8mMM+Fgzqdp6J+9BwAAAAAeMEigAAAAAOj2P+n5SvD0Amrc4BYc6Zo8n6i6idQPeJdfwuvX+FVbAAAAAVpBUgAAAAAAYMFhd/tsasSTOh2n7pqjC0DKe4SrMrNlFko7Z+3Ey0MAAAABr0QNgAAAAAA="
				horizonResponse := horizon.TransactionSuccess{
					Hash:      "be2765c309ab6911fe3938de0053672ef541290333a59dfb750f07919e9d6fec",
					Ledger:    &ledger,
					ResultXdr: &resultXdr,
					Extras:    nil,
				}

				mockTransactionSubmitter.On(
					"SubmitTransaction",
					mock.AnythingOfType("*string"),
					"SDWLS4G3XCNIYPKXJWWGGJT6UDY63WV6PEFTWP7JZMQB4RE7EUJQN5XM",
					mock.AnythingOfType("build.PaymentBuilder"),
					nil,
				).Run(func(args mock.Arguments) {
					operation, ok := args.Get(2).(build.PaymentBuilder)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, "GDSIKW43UA6JTOA47WVEBCZ4MYC74M3GNKNXTVDXFHXYYTNO5GGVN632", operation.PP.Destination.Address())
					assert.Equal(t, int64(1000000000), int64(operation.PP.SendMax))
					assert.Equal(t, int64(200000000), int64(operation.PP.DestAmount))
					assert.Equal(t, xdr.AssetTypeAssetTypeCreditAlphanum4, operation.PP.SendAsset.Type)
					assert.Equal(t, xdr.AssetTypeAssetTypeCreditAlphanum4, operation.PP.DestAsset.Type)
					// Path
					assert.Equal(t, xdr.AssetTypeAssetTypeNative, operation.PP.Path[0].Type)
					assert.Equal(t, xdr.AssetTypeAssetTypeCreditAlphanum4, operation.PP.Path[1].Type)
					assert.Equal(t, "GAF3PBFQLH57KPECN4GRGHU5NUZ3XXKYYWLOTBIRJMBYHPUBWANIUCZU", operation.PP.Path[1].AlphaNum4.Issuer.Address())
				}).Return(horizonResponse, nil).Once()

				Convey("it should return success", func() {
					statusCode, response := net.GetResponse(testServer, validParams)
					responseString := strings.TrimSpace(string(response))

					assert.Equal(t, 200, statusCode)
					expected := test.StringToJSONMap(`{
					  "hash": "be2765c309ab6911fe3938de0053672ef541290333a59dfb750f07919e9d6fec",
					  "ledger": 1988727,
					  "send_amount": "50.6480800",
					  "result_xdr": "AAAAAAAAAGQAAAAAAAAAAQAAAAAAAAACAAAAAAAAAAEAAAAAC8RjSvPMPWeQWzLq8JEM0BQNo0TfJQN/RwkCeJ+rT+YAAAAAAAAAAwAAAAFaQVIAAAAAAGDBYXf7bGrEkzodp+6aowtAynuEqzKzZRZKO2ftxMtDAAAAAa9EDYAAAAABVVNEAAAAAABstavC6cvn5h86pWOK5996Ape9k8mMM+Fgzqdp6J+9BwAAAAAeMEigAAAAAOj2P+n5SvD0Amrc4BYc6Zo8n6i6idQPeJdfwuvX+FVbAAAAAVpBUgAAAAAAYMFhd/tsasSTOh2n7pqjC0DKe4SrMrNlFko7Z+3Ey0MAAAABr0QNgAAAAAA="
					}`)
					assert.Equal(t, expected, test.StringToJSONMap(responseString))
				})
			})
		})
	})

	Convey("Given payment compliance request", t, func() {
		Convey("When params are valid", func() {
			params := url.Values{
				// GAW77Z6GPWXSODJOMF5L5BMX6VMYGEJRKUNBC2CZ725JTQZORK74HQQD
				"source":       {"SARMR3N465GTEHQLR3TSHDD7FHFC2I22ECFLYCHAZDEJWBVED66RW7FQ"},
				"sender":       {"alice*stellar.org"}, // GAW77Z6GPWXSODJOMF5L5BMX6VMYGEJRKUNBC2CZ725JTQZORK74HQQD
				"destination":  {"bob*stellar.org"},   // GAMVF7G4GJC4A7JMFJWLUAEIBFQD5RT3DCB5DC5TJDEKQBBACQ4JZVEE
				"amount":       {"20"},
				"asset_code":   {"USD"},
				"asset_issuer": {"GAMVF7G4GJC4A7JMFJWLUAEIBFQD5RT3DCB5DC5TJDEKQBBACQ4JZVEE"},
				"extra_memo":   {"hello world"},
			}

			Convey("it should return error when compliance server returns error", func() {
				mockHTTPClient.On(
					"PostForm",
					"http://compliance/send",
					mock.AnythingOfType("url.Values"),
				).Return(
					net.BuildHTTPResponse(400, "error"),
					nil,
				).Run(func(args mock.Arguments) {
					values, ok := args.Get(1).(url.Values)
					assert.True(t, ok, "Invalid conversion")
					// bridge server does not send source seed to compliance
					assert.Equal(t, []string{"GAW77Z6GPWXSODJOMF5L5BMX6VMYGEJRKUNBC2CZ725JTQZORK74HQQD"}, values["source"])
					values.Del("source")
					params.Del("source")
					assert.Equal(t, values.Encode(), params.Encode())
				}).Once()

				statusCode, response := net.GetResponse(testServer, params)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 500, statusCode)
				expected := test.StringToJSONMap(`{
  "code": "internal_server_error",
  "message": "Internal Server Error, please try again."
}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString))
			})

			Convey("it should return denied when compliance server returns denied", func() {
				mockHTTPClient.On(
					"PostForm",
					"http://compliance/send",
					mock.AnythingOfType("url.Values"),
				).Return(
					net.BuildHTTPResponse(200, "{\"auth_response\": {\"tx_status\": \"denied\"}}"),
					nil,
				).Run(func(args mock.Arguments) {
					values, ok := args.Get(1).(url.Values)
					assert.True(t, ok, "Invalid conversion")
					// bridge server does not send source seed to compliance
					assert.Equal(t, []string{"GAW77Z6GPWXSODJOMF5L5BMX6VMYGEJRKUNBC2CZ725JTQZORK74HQQD"}, values["source"])
					values.Del("source")
					params.Del("source")
					assert.Equal(t, values.Encode(), params.Encode())
				}).Once()

				statusCode, response := net.GetResponse(testServer, params)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 403, statusCode)
				expected := test.StringToJSONMap(`{
  "code": "denied",
  "message": "Transaction denied by destination."
}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString))
			})

			Convey("it should return pending when compliance server returns pending", func() {
				mockHTTPClient.On(
					"PostForm",
					"http://compliance/send",
					mock.AnythingOfType("url.Values"),
				).Return(
					net.BuildHTTPResponse(200, "{\"auth_response\": {\"info_status\": \"pending\", \"pending\": 3600}}"),
					nil,
				).Run(func(args mock.Arguments) {
					values, ok := args.Get(1).(url.Values)
					assert.True(t, ok, "Invalid conversion")
					// bridge server does not send source seed to compliance
					assert.Equal(t, []string{"GAW77Z6GPWXSODJOMF5L5BMX6VMYGEJRKUNBC2CZ725JTQZORK74HQQD"}, values["source"])
					values.Del("source")
					params.Del("source")
					assert.Equal(t, values.Encode(), params.Encode())
				}).Once()

				statusCode, response := net.GetResponse(testServer, params)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 202, statusCode)
				expected := test.StringToJSONMap(`{
  "code": "pending",
  "message": "Transaction pending. Repeat your request after given time.",
  "data": {
    "pending": 3600
  }
}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString))
			})

			Convey("it should submit transaction when compliance server returns success", func() {
				memoBytes, _ := hex.DecodeString("b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9")
				var hashXdr xdr.Hash
				copy(hashXdr[:], memoBytes[:])
				memo, _ := xdr.NewMemo(xdr.MemoTypeMemoHash, hashXdr)

				sourceBytes, _ := hex.DecodeString("2dffe7c67daf270d2e617abe8597f559831131551a116859feba99c32e8abfc3")
				var sourceXdr xdr.Uint256
				copy(sourceXdr[:], sourceBytes[:])

				destinationBytes, _ := hex.DecodeString("1952fcdc3245c07d2c2a6cba008809603ec67b1883d18bb348c8a8042014389c")
				var destinationXdr xdr.Uint256
				copy(destinationXdr[:], destinationBytes[:])

				issuerBytes, _ := hex.DecodeString("1952fcdc3245c07d2c2a6cba008809603ec67b1883d18bb348c8a8042014389c")
				var issuerXdr xdr.Uint256
				copy(issuerXdr[:], issuerBytes[:])

				expectedTx := &xdr.Transaction{
					SourceAccount: xdr.AccountId{
						Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
						Ed25519: &sourceXdr,
					},
					Fee:    100,
					SeqNum: 0,
					Memo:   memo,
					Operations: []xdr.Operation{
						{
							Body: xdr.OperationBody{
								Type: xdr.OperationTypePayment,
								PaymentOp: &xdr.PaymentOp{
									Destination: xdr.AccountId{
										Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
										Ed25519: &destinationXdr,
									},
									Amount: 200000000,
									Asset: xdr.Asset{
										Type: xdr.AssetTypeAssetTypeCreditAlphanum4,
										AlphaNum4: &xdr.AssetAlphaNum4{
											AssetCode: [4]byte{'U', 'S', 'D', 0},
											Issuer: xdr.AccountId{
												Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
												Ed25519: &issuerXdr,
											},
										},
									},
								},
							},
						},
					},
				}

				complianceResponse := callback.SendResponse{
					TransactionXdr: "AAAAAC3/58Z9rycNLmF6voWX9VmDETFVGhFoWf66mcMuir/DAAAAZAAAAAAAAAAAAAAAAAAAAAO5TSe5k00+CKUuUtfafav6xITv43pTgO6QiPes4u/N6QAAAAEAAAAAAAAAAQAAAAAZUvzcMkXAfSwqbLoAiAlgPsZ7GIPRi7NIyKgEIBQ4nAAAAAFVU0QAAAAAABlS/NwyRcB9LCpsugCICWA+xnsYg9GLs0jIqAQgFDicAAAAAAvrwgAAAAAA",
				}

				mockHTTPClient.On(
					"PostForm",
					"http://compliance/send",
					mock.AnythingOfType("url.Values"),
				).Return(
					net.BuildHTTPResponse(200, string(complianceResponse.Marshal())),
					nil,
				).Run(func(args mock.Arguments) {
					values, ok := args.Get(1).(url.Values)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, []string{"GAW77Z6GPWXSODJOMF5L5BMX6VMYGEJRKUNBC2CZ725JTQZORK74HQQD"}, values["source"])
					values.Del("source")
					params.Del("source")
					assert.Equal(t, values.Encode(), params.Encode())
				}).Once()

				var ledger uint64
				ledger = 1988727
				horizonResponse := horizon.TransactionSuccess{
					Hash:   "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
					Ledger: &ledger,
					Extras: nil,
				}

				mockTransactionSubmitter.On(
					"SignAndSubmitRawTransaction",
					mock.AnythingOfType("*string"),
					params.Get("source"),
					mock.AnythingOfType("*xdr.Transaction"),
				).Run(func(args mock.Arguments) {
					tx, ok := args.Get(2).(*xdr.Transaction)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, *tx, *expectedTx)
				}).Return(horizonResponse, nil).Once()

				statusCode, response := net.GetResponse(testServer, params)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 200, statusCode)
				expected := test.StringToJSONMap(`{
				  "hash": "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
				  "ledger": 1988727
				}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString))
			})

			Convey("it should return error when transaction submitter fails", func() {
				complianceResponse := callback.SendResponse{
					TransactionXdr: "AAAAAC3/58Z9rycNLmF6voWX9VmDETFVGhFoWf66mcMuir/DAAAAZAAAAAAAAAAAAAAAAAAAAAO5TSe5k00+CKUuUtfafav6xITv43pTgO6QiPes4u/N6QAAAAEAAAAAAAAAAQAAAAAZUvzcMkXAfSwqbLoAiAlgPsZ7GIPRi7NIyKgEIBQ4nAAAAAFVU0QAAAAAABlS/NwyRcB9LCpsugCICWA+xnsYg9GLs0jIqAQgFDicAAAAAAvrwgAAAAAA",
				}

				mockHTTPClient.On(
					"PostForm",
					"http://compliance/send",
					mock.AnythingOfType("url.Values"),
				).Return(
					net.BuildHTTPResponse(200, string(complianceResponse.Marshal())),
					nil,
				).Once()

				mockTransactionSubmitter.On(
					"SignAndSubmitRawTransaction",
					mock.AnythingOfType("*string"),
					mock.AnythingOfType("string"),
					mock.AnythingOfType("*xdr.Transaction"),
				).Return(
					horizon.TransactionSuccess{},
					errors.New("Transaction submitter error"),
				).Once()

				statusCode, response := net.GetResponse(testServer, params)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 500, statusCode)
				expected := test.StringToJSONMap(`{
					"code": "internal_server_error",
					"message": "Internal Server Error, please try again."
				}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString))
			})

			Convey("it should submit transaction when compliance server returns success (forward federation request)", func() {
				params["forward_destination[domain]"] = []string{"stellar.org"}
				params["forward_destination[fields][federation_type]"] = []string{"bank_account"}
				params["forward_destination[fields][swift]"] = []string{"BOPBPHMM"}
				params["forward_destination[fields][acct]"] = []string{"2382376"}

				memoBytes, _ := hex.DecodeString("b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9")
				var hashXdr xdr.Hash
				copy(hashXdr[:], memoBytes[:])
				memo, _ := xdr.NewMemo(xdr.MemoTypeMemoHash, hashXdr)

				sourceBytes, _ := hex.DecodeString("2dffe7c67daf270d2e617abe8597f559831131551a116859feba99c32e8abfc3")
				var sourceXdr xdr.Uint256
				copy(sourceXdr[:], sourceBytes[:])

				destinationBytes, _ := hex.DecodeString("1952fcdc3245c07d2c2a6cba008809603ec67b1883d18bb348c8a8042014389c")
				var destinationXdr xdr.Uint256
				copy(destinationXdr[:], destinationBytes[:])

				issuerBytes, _ := hex.DecodeString("1952fcdc3245c07d2c2a6cba008809603ec67b1883d18bb348c8a8042014389c")
				var issuerXdr xdr.Uint256
				copy(issuerXdr[:], issuerBytes[:])

				expectedTx := &xdr.Transaction{
					SourceAccount: xdr.AccountId{
						Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
						Ed25519: &sourceXdr,
					},
					Fee:    100,
					SeqNum: 0,
					Memo:   memo,
					Operations: []xdr.Operation{
						{
							Body: xdr.OperationBody{
								Type: xdr.OperationTypePayment,
								PaymentOp: &xdr.PaymentOp{
									Destination: xdr.AccountId{
										Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
										Ed25519: &destinationXdr,
									},
									Amount: 200000000,
									Asset: xdr.Asset{
										Type: xdr.AssetTypeAssetTypeCreditAlphanum4,
										AlphaNum4: &xdr.AssetAlphaNum4{
											AssetCode: [4]byte{'U', 'S', 'D', 0},
											Issuer: xdr.AccountId{
												Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
												Ed25519: &issuerXdr,
											},
										},
									},
								},
							},
						},
					},
				}

				complianceResponse := callback.SendResponse{
					TransactionXdr: "AAAAAC3/58Z9rycNLmF6voWX9VmDETFVGhFoWf66mcMuir/DAAAAZAAAAAAAAAAAAAAAAAAAAAO5TSe5k00+CKUuUtfafav6xITv43pTgO6QiPes4u/N6QAAAAEAAAAAAAAAAQAAAAAZUvzcMkXAfSwqbLoAiAlgPsZ7GIPRi7NIyKgEIBQ4nAAAAAFVU0QAAAAAABlS/NwyRcB9LCpsugCICWA+xnsYg9GLs0jIqAQgFDicAAAAAAvrwgAAAAAA",
				}

				mockHTTPClient.On(
					"PostForm",
					"http://compliance/send",
					mock.AnythingOfType("url.Values"),
				).Return(
					net.BuildHTTPResponse(200, string(complianceResponse.Marshal())),
					nil,
				).Run(func(args mock.Arguments) {
					values, ok := args.Get(1).(url.Values)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, []string{"GAW77Z6GPWXSODJOMF5L5BMX6VMYGEJRKUNBC2CZ725JTQZORK74HQQD"}, values["source"])
					values.Del("source")
					params.Del("source")
					assert.Equal(t, values.Encode(), params.Encode())
				}).Once()

				var ledger uint64
				ledger = 1988727
				horizonResponse := horizon.TransactionSuccess{
					Hash:   "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
					Ledger: &ledger,
					Extras: nil,
				}

				mockTransactionSubmitter.On(
					"SignAndSubmitRawTransaction",
					mock.AnythingOfType("*string"),
					params.Get("source"),
					mock.AnythingOfType("*xdr.Transaction"),
				).Run(func(args mock.Arguments) {
					tx, ok := args.Get(2).(*xdr.Transaction)
					assert.True(t, ok, "Invalid conversion")
					assert.Equal(t, *tx, *expectedTx)
				}).Return(horizonResponse, nil).Once()

				statusCode, response := net.GetResponse(testServer, params)
				responseString := strings.TrimSpace(string(response))
				assert.Equal(t, 200, statusCode)
				expected := test.StringToJSONMap(`{
				  "hash": "6a0049b44e0d0341bd52f131c74383e6ccd2b74b92c829c990994d24bbfcfa7a",
				  "ledger": 1988727
				}`)
				assert.Equal(t, expected, test.StringToJSONMap(responseString))
			})
		})
	})
}
