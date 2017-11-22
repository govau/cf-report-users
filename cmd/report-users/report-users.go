package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"code.cloudfoundry.org/cli/plugin"
	"github.com/olekukonko/tablewriter"
)

// simpleClient is a simple CloudFoundry client
type simpleClient struct {
	// API url, ie "https://api.system.example.com"
	API string

	// Authorization header, ie "bearer eyXXXXX"
	Authorization string
}

// Get makes a GET request, where r is the relative path, and rv is json.Unmarshalled to
func (sc *simpleClient) Get(r string, rv interface{}) error {
	log.Printf("GET %s%s\n", sc.API, r)
	req, err := http.NewRequest(http.MethodGet, sc.API+r, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", sc.Authorization)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("bad status code")
	}

	return json.NewDecoder(resp.Body).Decode(rv)
}

// List makes a GET request, to list resources, where we will follow the "next_url"
// to page results, and calls "f" as a callback to process each resource found
func (sc *simpleClient) List(r string, f func(*resource) error) error {
	for r != "" {
		var res struct {
			NextURL   string `json:"next_url"`
			Resources []*resource
		}
		err := sc.Get(r, &res)
		if err != nil {
			return err
		}

		for _, rr := range res.Resources {
			err = f(rr)
			if err != nil {
				return err
			}
		}

		r = res.NextURL
	}
	return nil
}

// resource captures fields that we care about when
// retrieving data from CloudFoundry
type resource struct {
	Entity struct {
		Name               string // org, space
		SpacesURL          string `json:"spaces_url"`           // org
		UsersURL           string `json:"users_url"`            // org
		ManagersURL        string `json:"managers_url"`         // org, space
		BillingManagersURL string `json:"billing_managers_url"` // org
		AuditorsURL        string `json:"auditors_url"`         // org, space
		DevelopersURL      string `json:"developers_url"`       // space
		Admin              bool   // user
		Username           string // user
	}
}

type reportUsers struct{}

func (c *reportUsers) Run(cliConnection plugin.CliConnection, args []string) {
	if args[0] == "report-users" {
		if len(args) != 1 {
			fmt.Println("Expected no args")
			os.Exit(1)
		}

		at, err := cliConnection.AccessToken()
		if err != nil {
			log.Fatal(err)
		}

		api, err := cliConnection.ApiEndpoint()
		if err != nil {
			log.Fatal(err)
		}

		client := &simpleClient{
			API:           api,
			Authorization: at,
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Organization", "Space", "Username", "Role"})

		err = client.List("/v2/organizations", func(org *resource) error {
			for _, orgRole := range []struct {
				Role string
				URL  string
			}{
				//{"OrgUser", org.Entity.UsersURL}, // These don't appear to be meaningful
				{"OrgManager", org.Entity.ManagersURL},
				{"OrgBillingManager", org.Entity.BillingManagersURL},
				{"OrgAuditor", org.Entity.AuditorsURL},
			} {
				err = client.List(orgRole.URL, func(user *resource) error {
					table.Append([]string{org.Entity.Name, "n/a", user.Entity.Username, orgRole.Role})
					return nil
				})
				if err != nil {
					return err
				}
			}

			return client.List(org.Entity.SpacesURL, func(space *resource) error {
				for _, spaceRole := range []struct {
					Role string
					URL  string
				}{
					{"SpaceDeveloper", space.Entity.DevelopersURL},
					{"SpaceManager", space.Entity.ManagersURL},
					{"SpaceAuditor", space.Entity.AuditorsURL},
				} {
					err = client.List(spaceRole.URL, func(user *resource) error {
						table.Append([]string{org.Entity.Name, space.Entity.Name, user.Entity.Username, spaceRole.Role})
						return nil
					})
					if err != nil {
						return err
					}
				}
				return nil
			})
		})
		if err != nil {
			log.Fatal(err)
		}

		table.Render()
	}
}

func (c *reportUsers) GetMetadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name: "Report Users",
		Version: plugin.VersionType{
			Major: 0,
			Minor: 2,
			Build: 0,
		},
		MinCliVersion: plugin.VersionType{
			Major: 6,
			Minor: 7,
			Build: 0,
		},
		Commands: []plugin.Command{
			{
				Name:     "report-users",
				HelpText: "Report all users in installation",
				UsageDetails: plugin.Usage{
					Usage: "report-users\n   cf report-users",
				},
			},
		},
	}
}

func main() {
	plugin.Start(&reportUsers{})
}
