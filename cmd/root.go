// Package cmd contains descriptions and handlers for vpn-dns CLI.
package cmd

import (
	"bit-exporter/internal/api"
	"bit-exporter/internal/codec"
	"bit-exporter/internal/domain"
	"bit-exporter/internal/export"
	"bit-exporter/pkg/crypto"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// AppName represents app name.
const AppName = "bit-exporter"

// Version represents current app version.
var Version = "development"

var clientSecret string
var clientId string
var password string
var apiUrl string

var outFile string
var decrypt bool

func getEnvAssert(key string, target *string) {
	*target = os.Getenv(key)
	if len(*target) == 0 {
		log.Fatalf("$%v variable is not set", key)
	}
}

func init() {
	err := godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("Error loading .env file: %v", err)
	}
	getEnvAssert("BW_CLIENT_SECRET", &clientSecret)
	getEnvAssert("BW_CLIENT_ID", &clientId)
	getEnvAssert("BW_API_URL", &apiUrl)
	rootCmd.PersistentFlags().StringVarP(
		&outFile,
		"out-file",
		"o",
		"bit-export.json",
		"out file name (default is bit-export.json)",
	)
	rootCmd.PersistentFlags().BoolVarP(
		&decrypt,
		"decrypt",
		"d",
		false,
		"decrypt content (default is false)",
	)

}

func getState(apiUrl string, id string, secret string) (*domain.Sync, domain.Auth) {
	api, err := api.New(apiUrl, id, secret)
	if err != nil {
		log.Fatalf("Authorization error: %v", err)
	}
	sync, err := api.Sync()
	if err != nil {
		log.Fatalf("Synchronization error: %v", err)
	}
	return &sync, api.Auth
}

func getKeys(key, email, password string, auth domain.Auth) ([]byte, []byte) {
	userKey, err := crypto.CalculateUserKey(
		password,
		email,
		auth.Kdf,
		auth.KdfIterations,
		auth.KdfMemory,
		auth.KdfParallelism,
	)
	masterKey, keyMac, err := crypto.DecryptMasterKey([]byte(key), userKey)
	if err != nil {
		log.Fatalf("Master key decryption error: %v", err)
	}
	return masterKey, keyMac
}

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:     AppName,
	Version: Version,
	Short:   "The app that exports records from a Bitwarden-compatible server",
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("Obtaining data")
		sync, auth := getState(apiUrl, clientId, clientSecret)
		if decrypt {
			getEnvAssert("BW_PASSWORD", &password)
			log.Println("Decrypting data")
			key, mac := getKeys(auth.Key, sync.Profile.Email, password, auth)
			c := codec.New(key, mac)
			c.Decode(sync)
		}
		file := export.FromDomain(sync)
		if decrypt {
			file.Encrypted = false
		} else {
			file.Encrypted = true
			file.Key = &auth.Key
		}
		content, err := json.Marshal(&file)
		if err != nil {
			log.Fatalf("JSON marshall error: %v", err)
		}
		jsonContent := string(content)
		err = ioutil.WriteFile(outFile, []byte(jsonContent), 0644)
		if err != nil {
			log.Fatalf("File writing error: %v", err)
		}
		log.Println("File " + outFile + " is saved")
	},
}

// Execute is the main CLI entrypoint.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
