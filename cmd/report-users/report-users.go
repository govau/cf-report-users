package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"time"

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
	Metadata struct {
		UpdatedAt time.Time `json:"updated_at"` // buildpack
	} `json:"metadata"`
	Entity struct {
		Name               string    // org, space
		SpacesURL          string    `json:"spaces_url"`              // org
		UsersURL           string    `json:"users_url"`               // org
		ManagersURL        string    `json:"managers_url"`            // org, space
		BillingManagersURL string    `json:"billing_managers_url"`    // org
		AuditorsURL        string    `json:"auditors_url"`            // org, space
		DevelopersURL      string    `json:"developers_url"`          // space
		AppsURL            string    `json:"apps_url"`                // space
		BuildpackGUID      string    `json:"detected_buildpack_guid"` // app
		Buildpack          string    `json:"buildpack"`               // app
		Admin              bool      // user
		Username           string    // user
		Filename           string    `json:"filename"`           // buildpack
		Enabled            bool      `json:"enabled"`            // buildpack
		PackageUpdatedAt   time.Time `json:"package_updated_at"` // app
	} `json:"entity"`
}

type reportUsers struct{}

func newSimpleClient(cliConnection plugin.CliConnection) (*simpleClient, error) {
	at, err := cliConnection.AccessToken()
	if err != nil {
		return nil, err
	}

	api, err := cliConnection.ApiEndpoint()
	if err != nil {
		return nil, err
	}

	return &simpleClient{
		API:           api,
		Authorization: at,
	}, nil
}

func (c *reportUsers) Run(cliConnection plugin.CliConnection, args []string) {
	switch args[0] {
	case "report-users":
		if len(args) != 1 {
			log.Fatal("Expected no args")
		}
		err := c.reportUsers(cliConnection)
		if err != nil {
			log.Fatal(err)
		}

	case "report-buildpacks":
		if len(args) != 1 {
			log.Fatal("Expected no args")
		}
		err := c.reportBuildpacks(cliConnection)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func (c *reportUsers) reportBuildpacks(cliConnection plugin.CliConnection) error {
	client, err := newSimpleClient(cliConnection)
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Organization", "Space", "Application", "Buildpack", "Out of date"})

	buildpacks := make(map[string]*resource)

	err = client.List("/v2/organizations", func(org *resource) error {
		return client.List(org.Entity.SpacesURL, func(space *resource) error {
			return client.List(space.Entity.AppsURL, func(app *resource) error {
				bp, ok := buildpacks[app.Entity.BuildpackGUID]
				if !ok {
					var bbp resource
					err := client.Get(fmt.Sprintf("/v2/buildpacks/%s", app.Entity.BuildpackGUID), &bbp)
					if err != nil {
						bbp.Entity.Filename = app.Entity.Buildpack
						bbp.Entity.Enabled = false
					}
					buildpacks[app.Entity.BuildpackGUID] = &bbp
					bp = &bbp
				}
				ood := ""
				if bp.Metadata.UpdatedAt.After(app.Entity.PackageUpdatedAt) {
					ood = fmt.Sprintf("%d days", int(math.Ceil(bp.Metadata.UpdatedAt.Sub(app.Entity.PackageUpdatedAt).Hours()/24.0)))
				} else if !bp.Entity.Enabled {
					ood = "Needs attention"
				}
				table.Append([]string{org.Entity.Name, space.Entity.Name, app.Entity.Name, app.Entity.Buildpack, ood})
				return nil
			})
		})
	})
	if err != nil {
		return err
	}

	table.Render()
	return nil
}

func (c *reportUsers) reportUsers(cliConnection plugin.CliConnection) error {
	client, err := newSimpleClient(cliConnection)
	if err != nil {
		return err
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
		return err
	}

	table.Render()
	return nil
}

func (c *reportUsers) GetMetadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name: "Report Users",
		Version: plugin.VersionType{
			Major: 0,
			Minor: 3,
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
			{
				Name:     "report-buildpacks",
				HelpText: "Report all buildpacks used in installation",
				UsageDetails: plugin.Usage{
					Usage: "report-buildpacks\n   cf report-buildpacks",
				},
			},
		},
	}
}

func main() {
	plugin.Start(&reportUsers{})
}
