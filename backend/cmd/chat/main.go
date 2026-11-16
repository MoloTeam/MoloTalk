// Команда chat запускает процесс Chat.
package main

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/MoloTeam/MoloTalk/backend/internal/chat"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/bootstrap"
	"github.com/MoloTeam/MoloTalk/backend/internal/platform/config"
)

func main() {
	// Реализации портов состава и транзакций подключаются в T-02/T-03.
	module := chat.New(chat.Deps{})

	if err := bootstrap.Run(context.Background(), bootstrap.Options{
		Service: config.Chat,
		Hosted:  module,
		Getenv:  os.Getenv,
		Listen:  net.Listen,
		Out:     os.Stdout,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
