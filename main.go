package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/viper"

	"ai-hub-example/controller"
	"ai-hub-example/hostclient"
	"ai-hub-example/model"
	"ai-hub-example/processor"
	"ai-hub-example/router"
)

//go:embed web/build
var frontend embed.FS

//go:embed plugin.json
var manifest []byte

func main() {
	if err := run(); err != nil {
		slog.Error("example service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", "config.yaml", "configuration file")
	flag.Parse()
	v := viper.New()
	v.SetConfigFile(*path)
	v.SetEnvPrefix("EXAMPLE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("server.address", "127.0.0.1:3012")
	v.SetDefault("database.mode", "independent")
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.sqlite_path", "./data/example.db")
	v.SetDefault("database.sqlite_busy_timeout", 3000)
	v.SetDefault("host.base_url", "http://127.0.0.1:3000")
	if err := v.ReadInConfig(); err != nil && !os.IsNotExist(err) {
		return err
	}
	var info struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(manifest, &info); err != nil {
		return err
	}
	if id := v.GetString("plugin.id"); id != "" && id != info.ID {
		return errors.New("启动插件标识与嵌入清单不一致")
	}
	secret := v.GetString("host.secret")
	if len(secret) < 32 {
		return errors.New("host.secret 至少需要 32 字符")
	}
	var rules processor.Rules
	if err := v.UnmarshalKey("rules", &rules); err != nil {
		return err
	}
	if err := rules.Validate(); err != nil {
		return err
	}
	store, err := model.Open(v)
	if err != nil {
		return err
	}
	db, err := store.DB.DB()
	if err != nil {
		return err
	}
	defer db.Close()
	host, err := hostclient.New(v.GetString("host.base_url"), *path, v.GetString("host.data_base_url"))
	if err != nil {
		return err
	}
	assets, err := fs.Sub(frontend, "web/build")
	if err != nil {
		return err
	}
	service := &controller.Service{Store: store, Host: host, Rules: rules}
	server := &http.Server{Addr: v.GetString("server.address"), Handler: router.New(service, secret, assets, manifest), ReadHeaderTimeout: 10 * time.Second}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-ctx.Done()
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = server.Shutdown(shutdown)
	}()
	slog.Info("example service listening", "address", server.Addr)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
