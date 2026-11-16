// Команда media запускает процесс Media.
package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/MoloTeam/MoloTalk/backend/internal/media"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/bootstrap"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
)

func main() {
	// Реализации портов сообщений, вложений, подтверждений и обмена событиями
	// подключаются в T-02/T-03.
	module := media.New(media.Deps{})

	if err := bootstrap.Run(context.Background(), bootstrap.Options{
		Service: config.Media,
		Hosted:  module,
		Getenv:  os.Getenv,
		Listen:  net.Listen,
		Out:     os.Stdout,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
