package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"

	"github.com/logerror/t2t/pkg/constants/svcconstants"
	"github.com/logerror/t2t/pkg/data/common"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginAuth struct {
	Username string
	Password string
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "login to the t2t server, use username and password",
	Run: func(cmd *cobra.Command, args []string) {
		if loginAuth.Username == "" {
			fmt.Printf("username: ")
			fmt.Scanln(&loginAuth.Username)
		}
		if loginAuth.Password == "" {
			fmt.Printf("password: ")
			password, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				panic(err)
			}
			fmt.Println()
			loginAuth.Password = string(password)
		}

		data, err := json.Marshal(common.LoginInput{User: loginAuth.Username, Password: loginAuth.Password})
		if err != nil {
			panic(err)
		}
		loginUrl := fmt.Sprintf("%s://%s/api/login",
			svcconstants.AgentServerHttpSchema,
			svcconstants.AgentServerHost)

		resp, err := http.Post(loginUrl, "application/json", bytes.NewReader(data))
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()

		var output common.LoginOutput
		if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
			panic(err)
		}

		if output.Error != "" {
			fmt.Printf("error(%s): login failed. please try again.\n%s\n", resp.Status, output.Error)
			os.Exit(1)
		}

		home, err := os.UserHomeDir()
		if err != nil {
			panic(err)
		}
		os.MkdirAll(filepath.Join(home, ".config", "t2t"), 0755)
		if err := os.WriteFile(filepath.Join(home, ".config", "t2t", "auth.token"), []byte(output.Token), 0644); err != nil {
			panic(err)
		}

		fmt.Println("login successfully")
	},
}

func init() {
	loginCmd.Flags().StringVarP(&loginAuth.Username, "username", "u", "", "username for login")
	loginCmd.Flags().StringVarP(&loginAuth.Password, "password", "p", "", "password for login")
	rootCmd.AddCommand(loginCmd)
}
