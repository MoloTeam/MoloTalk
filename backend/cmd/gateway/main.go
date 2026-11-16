// Команда gateway запускает процесс Gateway.
package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/MoloTeam/MoloTalk/backend/internal/gateway"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/bootstrap"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
)

func main() {
	// Реализации межсервисных клиентов и подписки на события подключаются
	// в T-02/T-03.
	module := gateway.New(gateway.Deps{})

	if err := bootstrap.Run(context.Background(), bootstrap.Options{
		Service: config.Gateway,
		Hosted:  module,
		Getenv:  os.Getenv,
		Listen:  net.Listen,
		Out:     os.Stdout,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
