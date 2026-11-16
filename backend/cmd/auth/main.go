// Команда auth запускает процесс Auth.
package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/MoloTeam/MoloTalk/backend/internal/auth"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/bootstrap"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
)

func main() {
	// Реализации портов хранения, хеширования и токенов подключаются в T-02/T-03.
	module := auth.New(auth.Deps{})

	if err := bootstrap.Run(context.Background(), bootstrap.Options{
		Service: config.Auth,
		Hosted:  module,
		Getenv:  os.Getenv,
		Listen:  net.Listen,
		Out:     os.Stdout,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
