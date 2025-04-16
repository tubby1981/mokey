package serve

import (
	"context"
	"os"
	"os/signal"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tubby1981/mokey/cmd"
	"github.com/tubby1981/mokey/server"
)

var (
	serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Run server",
		Long:  `Run server`,
		RunE: func(command *cobra.Command, args []string) error {
			return serve()
		},
	}
)

func init() {
	serveCmd.Flags().String("keytab", "", "path to keytab file")
	viper.BindPFlag("site.keytab", serveCmd.Flags().Lookup("keytab"))

	serveCmd.Flags().String("listen", "0.0.0.0:8866", "address to listen on")
	viper.BindPFlag("server.listen", serveCmd.Flags().Lookup("listen"))
	serveCmd.Flags().String("cert", "", "path to ssl cert")
	viper.BindPFlag("server.ssl_cert", serveCmd.Flags().Lookup("cert"))
	serveCmd.Flags().String("key", "", "path to ssl key")
	viper.BindPFlag("server.ssl_key", serveCmd.Flags().Lookup("key"))
	serveCmd.Flags().String("dbpath", "", "path to mokey database")
	viper.BindPFlag("storage.sqlite3.dbpath", serveCmd.Flags().Lookup("dbpath"))

	serveCmd.Flags().String("smtp-host", "localhost", "smtp host")
	viper.BindPFlag("email.smtp_host", serveCmd.Flags().Lookup("smtp-host"))

	serveCmd.Flags().Int("smtp-port", 25, "smtp port")
	viper.BindPFlag("email.smtp_port", serveCmd.Flags().Lookup("smtp-port"))

	serveCmd.Flags().String("email-from", "", "from email address")
	viper.BindPFlag("email.from", serveCmd.Flags().Lookup("email-from"))

	cmd.Root.AddCommand(serveCmd)
}

func serve() error {
	srv, err := server.NewServer(viper.GetString("server.listen"))
	if err != nil {
		return err
	}

	srv.KeyFile = viper.GetString("server.ssl_key")
	srv.CertFile = viper.GetString("server.ssl_cert")

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt)
		<-quit
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		logrus.Debug("Shutting down server")
		srv.Shutdown(ctx)
	}()

	return srv.Serve()
}
