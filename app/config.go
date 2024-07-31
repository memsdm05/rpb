package app

import (
	"flag"
	"log"
	"os"
	"time"
)

var Config = struct {
	DBPath     string
	Secret     string
	Addr       string
	Timeout    time.Duration
	PinInput   int
	PinOutput  int
	Production bool
}{}

func LoadConfig() {
	// viper.SetEnvPrefix("rpb")
	// viper.AutomaticEnv()

	// err := viper.Unmarshal(&Config)
	// if err != nil {
	// 	panic(err)
	// }

	flag.StringVar(&Config.Secret, "secret", "", "Login password (required)")
	flag.StringVar(&Config.DBPath, "db", "./rpb.db", "Where the database is")
	flag.StringVar(&Config.Addr, "addr", ":5000", "Address to bind to")
	flag.IntVar(&Config.PinInput, "input", 14, "Pin used for input")
	flag.IntVar(&Config.PinOutput, "output", 15, "Pin used for output")
	flag.BoolVar(&Config.Production, "prod", false, "Tells server to actually activate pins")
	flag.DurationVar(&Config.Timeout, "timeout", 20*time.Second, "Maximum time a server waits before releasing a button")

	flag.Parse()

	if Config.Secret == "" {
		Config.Secret = os.Getenv("RPB_SECRET")
	}
	if Config.Secret == "" || Config.Secret == "<INSERT SECRET HERE>" {
		log.Fatalln("secret must be supplied")
	}
}
