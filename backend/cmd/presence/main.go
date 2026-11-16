// Команда presence запускает процесс Presence.
package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/MoloTeam/MoloTalk/backend/internal/platform/bootstrap"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
	"github.com/MoloTeam/MoloTalk/backend/internal/presence"
)

func main() {
	// Реализации состояния присутствия и межсервисных клиентов подключаются
	// в T-02/T-03.
	module := presence.New(presence.Deps{})

	if err := bootstrap.Run(context.Background(), bootstrap.Options{
		Service: config.Presence,
		Hosted:  module,
		Getenv:  os.Getenv,
		Listen:  net.Listen,
		Out:     os.Stdout,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
