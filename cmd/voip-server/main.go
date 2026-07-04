package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/pbx"
	"github.com/sappsys/VoIP_Server/internal/store"
	"github.com/sappsys/VoIP_Server/internal/version"
	"github.com/sappsys/VoIP_Server/internal/web"
)

func main() {
	cfgPath := flag.String("config", "config.toml", "path to config.toml")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Version)
		return
	}

	cfg, err := config.LoadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	log := config.NewLogger(cfg.Logging)
	log.Info("starting VoIP PBX", "version", version.Version, "log_level", cfg.Logging.Level)

	cfgDir := filepath.Dir(*cfgPath)
	if cfgDir == "." {
		cfgDir, _ = os.Getwd()
	}

	dbPath := cfg.Database.Path
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(cfgDir, dbPath)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Error("mkdir data", "error", err)
		os.Exit(1)
	}

	extDir := cfg.Paths.ExtensionsDir
	if !filepath.IsAbs(extDir) {
		extDir = filepath.Join(cfgDir, extDir)
	}

	phonebookDir := cfg.Paths.PhonebookDir
	if !filepath.IsAbs(phonebookDir) {
		phonebookDir = filepath.Join(cfgDir, phonebookDir)
	}
	_ = os.MkdirAll(phonebookDir, 0o755)

	st, err := store.Open(dbPath)
	if err != nil {
		log.Error("open store", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	for _, u := range cfg.Users {
		hash, err := store.HashPassword(u.Password)
		if err != nil {
			log.Error("hash web user", "user", u.Username, "error", err)
			continue
		}
		role := u.Role
		if role == "" {
			role = "admin"
		}
		_ = st.UpsertWebUser(u.Username, hash, role)
	}

	exts, err := config.LoadExtensions(extDir, cfg.Limits.MaxCallsPerExtension)
	if err != nil {
		log.Error("load extensions", "error", err)
		os.Exit(1)
	}

	pbxSrv, err := pbx.New(cfg, cfgDir, extDir, exts, st, log)
	if err != nil {
		log.Error("pbx init", "error", err)
		os.Exit(1)
	}
	defer pbxSrv.Close()

	webSrv := web.New(cfg, *cfgPath, extDir, phonebookDir, st, pbxSrv, log)
	httpSrv := &http.Server{Addr: cfg.Web.Listen, Handler: webSrv.Handler()}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	go func() {
		log.Info("web listening", "addr", cfg.Web.Listen)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("web server", "error", err)
			cancel()
		}
	}()

	go func() {
		log.Info("sip listening", "addr", cfg.SIPBindHost(), "port", cfg.Server.BindPort, "transports", cfg.Server.SIPTransports())
		if cfg.NAT.SIPProxyEnabled {
			log.Info("sip nat proxy", "port", cfg.NAT.SIPProxyPort)
		}
		if cfg.NAT.STUNEnabled {
			log.Info("stun enabled", "addr", cfg.STUNBindHost(), "port", cfg.NAT.STUNPort)
		}
		if err := pbxSrv.Run(ctx); err != nil && ctx.Err() == nil {
			log.Error("sip server", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	_ = httpSrv.Shutdown(context.Background())
	log.Info("shutdown complete")
}
