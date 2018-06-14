package main

import (
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/facebookgo/inject"
	"github.com/spf13/cobra"
	"github.com/stellar/go/clients/federation"
	"github.com/stellar/go/clients/horizon"
	"github.com/stellar/go/clients/stellartoml"
	"github.com/stellar/go/services/bridge/internal/config"
	"github.com/stellar/go/services/bridge/internal/db"
	"github.com/stellar/go/services/bridge/internal/handlers"
	"github.com/stellar/go/services/bridge/internal/listener"
	"github.com/stellar/go/services/bridge/internal/submitter"
	supportConfig "github.com/stellar/go/support/config"
	"github.com/stellar/go/support/errors"
	supportHttp "github.com/stellar/go/support/http"
	"github.com/zenazn/goji/graceful"
	"github.com/zenazn/goji/web"
	"github.com/zenazn/goji/web/middleware"
)

var app *App
var rootCmd *cobra.Command
var migrateFlag bool
var configFile string
var versionFlag bool
var version = "N/A"

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	rootCmd.Execute()
}

func init() {
	rootCmd = &cobra.Command{
		Use:   "bridge",
		Short: "stellar bridge server",
		Long:  `stellar bridge server`,
		Run:   run,
	}

	rootCmd.Flags().BoolVarP(&migrateFlag, "migrate-db", "", false, "migrate DB to the newest schema version")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "bridge.cfg", "path to config file")
	rootCmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "displays bridge server version")
}

func run(cmd *cobra.Command, args []string) {
	var cfg config.Config

	err := supportConfig.Read(configFile, &cfg)
	if err != nil {
		switch cause := errors.Cause(err).(type) {
		case *supportConfig.InvalidConfigError:
			log.Error("config file: ", cause)
		default:
			log.Error(err)
		}
		os.Exit(-1)
	}

	err = cfg.Validate()
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	if cfg.LogFormat == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	}

	app, err = NewApp(cfg, migrateFlag, versionFlag, version)

	if err != nil {
		log.Fatal(err.Error())
		return
	}

	app.Serve()
}

// App is the application object
type App struct {
	config         config.Config
	requestHandler handlers.RequestHandler
}

// NewApp constructs an new App instance from the provided config.
func NewApp(config config.Config, migrateFlag bool, versionFlag bool, version string) (app *App, err error) {
	var g inject.Graph

	var database db.PostgresDatabase

	if config.Database.URL != "" {
		err = database.Open(config.Database.URL)
		if err != nil {
			err = fmt.Errorf("Cannot connect to a DB: %s", err)
			return
		}
	}

	if migrateFlag {
		// TODO
		// if driver == nil {
		// 	log.Fatal("No database driver.")
		// 	return
		// }

		// var migrationsApplied int
		// migrationsApplied, err = driver.MigrateUp("gateway")
		// if err != nil {
		// 	return
		// }

		// log.Info("Applied migrations: ", migrationsApplied)
		// os.Exit(0)
		// return
	}

	if versionFlag {
		fmt.Printf("Bridge Server Version: %s \n", version)
		os.Exit(0)
		return
	}

	if len(config.APIKey) > 0 && len(config.APIKey) < 15 {
		err = errors.New("api-key have to be at least 15 chars long")
		return
	}

	requestHandler := handlers.RequestHandler{}

	httpClientWithTimeout := http.Client{
		Timeout: 60 * time.Second,
	}

	h := horizon.Client{
		URL:  config.Horizon,
		HTTP: &httpClientWithTimeout,
	}

	log.Print("Creating and initializing TransactionSubmitter")
	ts := submitter.NewTransactionSubmitter(&h, &database, config.NetworkPassphrase, time.Now)
	if err != nil {
		return
	}

	if config.Accounts.AuthorizingSeed == "" {
		log.Warning("No accounts.authorizing_seed param. Skipping...")
	} else {
		log.Print("Initializing Authorizing account")
		err = ts.InitAccount(config.Accounts.AuthorizingSeed)
		if err != nil {
			return
		}
	}

	if config.Accounts.BaseSeed == "" {
		log.Warning("No accounts.base_seed param. Skipping...")
	} else {
		log.Print("Initializing Base account")
		err = ts.InitAccount(config.Accounts.BaseSeed)
		if err != nil {
			return
		}
	}

	log.Print("TransactionSubmitter created")

	log.Print("Creating and starting PaymentListener")

	var paymentListener listener.PaymentListener

	if config.Accounts.ReceivingAccountID == "" {
		log.Warning("No accounts.receiving_account_id param. Skipping...")
	} else if config.Callbacks.Receive == "" {
		log.Warning("No callbacks.receive param. Skipping...")
	} else {
		paymentListener, err = listener.NewPaymentListener(&config, &database, &h, time.Now)
		if err != nil {
			return
		}
		err = paymentListener.Listen()
		if err != nil {
			return
		}

		log.Print("PaymentListener created")
	}

	stellartomlClient := stellartoml.Client{
		HTTP: &httpClientWithTimeout,
	}

	federationClient := federation.Client{
		HTTP:        &httpClientWithTimeout,
		StellarTOML: &stellartomlClient,
	}

	err = g.Provide(
		&inject.Object{Value: &requestHandler},
		&inject.Object{Value: &config},
		&inject.Object{Value: &stellartomlClient},
		&inject.Object{Value: &federationClient},
		&inject.Object{Value: &h},
		&inject.Object{Value: &database},
		&inject.Object{Value: &ts},
		&inject.Object{Value: &paymentListener},
		&inject.Object{Value: &httpClientWithTimeout},
	)

	if err != nil {
		log.Fatal("Injector: ", err)
	}

	if err := g.Populate(); err != nil {
		log.Fatal("Injector: ", err)
	}

	app = &App{
		config:         config,
		requestHandler: requestHandler,
	}
	return
}

// Serve starts the server
func (a *App) Serve() {
	portString := fmt.Sprintf(":%d", *a.config.Port)
	flag.Set("bind", portString)

	bridge := web.New()

	bridge.Abandon(middleware.Logger)

	// Middlewares
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	bridge.Use(supportHttp.StripTrailingSlashMiddleware("/admin"))
	bridge.Use(supportHttp.HeadersMiddleware(headers, "/admin/"))

	if a.config.APIKey != "" {
		bridge.Use(apiKeyMiddleware(a.config.APIKey))
	}

	if a.config.Accounts.AuthorizingSeed != "" {
		bridge.Post("/authorize", a.requestHandler.Authorize)
	} else {
		log.Warning("accounts.authorizing_seed not provided. /authorize endpoint will not be available.")
	}

	bridge.Post("/create-keypair", a.requestHandler.CreateKeypair)
	bridge.Post("/builder", a.requestHandler.Builder)
	bridge.Post("/payment", a.requestHandler.Payment)
	bridge.Get("/payment", a.requestHandler.Payment)
	bridge.Post("/reprocess", a.requestHandler.Reprocess)

	bridge.Get("/admin/received-payments", a.requestHandler.AdminReceivedPayments)
	bridge.Get("/admin/received-payments/:id", a.requestHandler.AdminReceivedPayment)
	bridge.Get("/admin/sent-transactions", a.requestHandler.AdminSentTransactions)

	err := graceful.ListenAndServe(portString, bridge)
	if err != nil {
		log.Fatal(err)
	}
}

// apiKeyMiddleware checks for apiKey in a request and writes http.StatusForbidden if it's incorrect.
func apiKeyMiddleware(apiKey string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			k := r.PostFormValue("apiKey")
			if k != apiKey {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}
