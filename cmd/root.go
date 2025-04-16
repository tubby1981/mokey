package cmd

import (
	"fmt"
	"io/ioutil"
	golog "log"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tubby1981/mokey/server"
)

var (
	cfgFile     string
	cfgFileUsed string
	logLevel    string

	Root = &cobra.Command{
		Use:     "mokey",
		Version: server.Version,
		Short:   "FreeIPA self-service account management tool",
		Long:    ``,
	}
)

func Execute() {
	if err := Root.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	Root.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file")
	Root.PersistentFlags().StringVar(&logLevel, "loglevel", "info", "Set log level")

	Root.PersistentPreRunE = func(command *cobra.Command, args []string) error {
		return SetupLogging()
	}
}

func SetupLogging() error {
	switch logLevel {
	case "trace":
		logrus.SetLevel(logrus.TraceLevel)
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		return fmt.Errorf("Unknown log level: %s", logLevel)
	}

	golog.SetOutput(ioutil.Discard)

	if cfgFileUsed != "" {
		logrus.Infof("Using config file: %s", cfgFileUsed)
	}

	Root.SilenceUsage = true
	Root.SilenceErrors = true

	return nil
}

func initConfig() {
	viper.SetConfigType("toml")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			logrus.Fatal(err)
		}

		viper.AddConfigPath("/etc/mokey/")
		viper.AddConfigPath(cwd)
		viper.SetConfigName("mokey.toml")
	}

	server.SetDefaults()
	viper.AutomaticEnv()
	viper.SetEnvPrefix("mokey")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			logrus.Fatalf("Failed parsing config file %s: %s", viper.ConfigFileUsed(), err)
		}
	} else {
		cfgFileUsed = viper.ConfigFileUsed()
	}

	if !viper.IsSet("email.token_secret") {
		secret, err := server.GenerateSecret(32)
		if err != nil {
			logrus.Fatal(err)
		}

		viper.Set("email.token_secret", secret)
	}

	if !viper.IsSet("server.csrf_secret") {
		secret, err := server.GenerateSecretString(16)
		if err != nil {
			logrus.Fatal(err)
		}

		viper.Set("server.csrf_secret", secret)
	}

	if !viper.IsSet("site.keytab") {
		logrus.Fatalf("Please provide path to keytab file")
	}

	if viper.IsSet("accounts.otp_hash_algorithm") {
		algo := viper.GetString("accounts.otp_hash_algorithm")
		validAlgo := false
		for _, a := range []string{"sha1", "sha256", "sha512"} {
			if algo == a {
				validAlgo = true
				break
			}
		}

		if !validAlgo {
			logrus.Fatalf("Invalid otp hash algorithm: %s", algo)
		}
	}
}
