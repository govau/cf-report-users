package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
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

	// Quiet - if set don't print progress to stderr
	Quiet bool
}

// Get makes a GET request, where r is the relative path, and rv is json.Unmarshalled to
func (sc *simpleClient) Get(r string, rv interface{}) error {
	if !sc.Quiet {
		log.Printf("GET %s%s", sc.API, r)
	}
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
		Guid      string    `json:"guid"`       // app
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

type droplet struct {
	Buildpacks []struct {
		Name          string `json:"name"`
		BuildpackName string `json:"buildpack_name"`
		Version       string `json:"version"`
	} `json:"buildpacks"`
}

type reportUsers struct{}

func newSimpleClient(cliConnection plugin.CliConnection, quiet bool) (*simpleClient, error) {
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
		Quiet:         quiet,
	}, nil
}

func (c *reportUsers) Run(cliConnection plugin.CliConnection, args []string) {
	outputJSON := false
	quiet := false

	fs := flag.NewFlagSet("report-users", flag.ExitOnError)
	fs.BoolVar(&outputJSON, "output-json", false, "if set sends JSON to stdout instead of a rendered table")
	fs.BoolVar(&quiet, "quiet", false, "if set suppressing printing of progress messages to stderr")
	err := fs.Parse(args[1:])
	if err != nil {
		log.Fatal(err)
	}

	client, err := newSimpleClient(cliConnection, quiet)
	if err != nil {
		log.Fatal(err)
	}

	switch args[0] {
	case "report-users":
		err := c.reportUsers(client, os.Stdout, outputJSON)
		if err != nil {
			log.Fatal(err)
		}

	case "report-buildpacks":
		err := c.reportBuildpacks(client, os.Stdout, outputJSON)
		if err != nil {
			log.Fatal(err)
		}
	}
}

type buildpackUsageInfo struct {
	Organization string   `json:"organization"`
	Space        string   `json:"space"`
	Application  string   `json:"application"`
	Buildpacks   []string `json:"buildpacks,omitempty"`
	Messages     []string `json:"messages,omitempty"`
}

func (c *reportUsers) reportBuildpacks(client *simpleClient, out io.Writer, outputJSON bool) error {
	buildpacks := make(map[string]*resource)
	err := client.List("/v2/buildpacks", func(bp *resource) error {
		if bp.Entity.Enabled {
			buildpacks[bp.Entity.Name] = bp
		}
		return nil
	})
	if err != nil {
		return err
	}

	var allInfo []*buildpackUsageInfo
	err = client.List("/v2/organizations", func(org *resource) error {
		return client.List(org.Entity.SpacesURL, func(space *resource) error {
			return client.List(space.Entity.AppsURL, func(app *resource) error {
				var bps []string
				var messages []string

				var dropletAnswer droplet
				err := client.Get(fmt.Sprintf("/v3/apps/%s/droplets/current", app.Metadata.Guid), &dropletAnswer)
				if err != nil {
					messages = append(messages, "needs attention (1)")
				} else {
					if len(dropletAnswer.Buildpacks) == 0 {
						messages = append(messages, "needs attention (2)")
					}
					for _, bp := range dropletAnswer.Buildpacks {
						if bp.Version == "" {
							messages = append(messages, "needs attention (3)")
						} else {
							bps = append(bps, fmt.Sprintf("%s v%s", bp.BuildpackName, bp.Version))
							bpr, found := buildpacks[bp.Name]
							if !found {
								messages = append(messages, "needs attention (4)")
							} else {
								if !strings.HasSuffix(bpr.Entity.Filename, fmt.Sprintf("v%s.zip", bp.Version)) {
									messages = append(messages, "needs attention (5)")
								}
							}
						}
					}
				}

				if len(bps) == 0 {
					if app.Entity.Buildpack != "" {
						bps = append(bps, app.Entity.Buildpack)
					}
				}

				if len(messages) == 0 {
					messages = append(messages, "OK")
				}

				allInfo = append(allInfo, &buildpackUsageInfo{
					Organization: org.Entity.Name,
					Space:        space.Entity.Name,
					Application:  app.Entity.Name,
					Buildpacks:   bps,
					Messages:     messages,
				})

				return nil
			})
		})
	})
	if err != nil {
		return err
	}

	if outputJSON {
		return json.NewEncoder(out).Encode(allInfo)
	}

	table := tablewriter.NewWriter(out)
	table.SetHeader([]string{"Organization", "Space", "Application", "Buildpacks", "Messages"})
	for _, row := range allInfo {
		table.Append([]string{
			row.Organization,
			row.Space,
			row.Application,
			strings.Join(row.Buildpacks, ", "),
			strings.Join(row.Messages, ", "),
		})
	}
	table.Render()

	return nil
}

type userInfoLineItem struct {
	Organization string `json:"organization"`
	Space        string `json:"space,omitempty"`
	Username     string `json:"username"`
	Role         string `json:"role"`
}

func (c *reportUsers) reportUsers(client *simpleClient, out io.Writer, outputJSON bool) error {
	var allInfo []*userInfoLineItem
	err := client.List("/v2/organizations", func(org *resource) error {
		for _, orgRole := range []struct {
			Role string
			URL  string
		}{
			//{"OrgUser", org.Entity.UsersURL}, // These don't appear to be terribly meaningful
			{"OrgManager", org.Entity.ManagersURL},
			{"OrgBillingManager", org.Entity.BillingManagersURL},
			{"OrgAuditor", org.Entity.AuditorsURL},
		} {
			err := client.List(orgRole.URL, func(user *resource) error {
				allInfo = append(allInfo, &userInfoLineItem{
					Organization: org.Entity.Name,
					Username:     user.Entity.Username,
					Role:         orgRole.Role,
				})
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
				err := client.List(spaceRole.URL, func(user *resource) error {
					allInfo = append(allInfo, &userInfoLineItem{
						Organization: org.Entity.Name,
						Space:        space.Entity.Name,
						Username:     user.Entity.Username,
						Role:         spaceRole.Role,
					})
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

	if outputJSON {
		return json.NewEncoder(out).Encode(allInfo)
	}

	table := tablewriter.NewWriter(out)
	table.SetHeader([]string{"Organization", "Space", "Username", "Role"})
	for _, info := range allInfo {
		table.Append([]string{info.Organization, info.Space, info.Username, info.Role})
	}
	table.Render()
	return nil
}

func (c *reportUsers) GetMetadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name: "Report Users",
		Version: plugin.VersionType{
			Major: 0,
			Minor: 5,
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
					Usage: "cf report-users",
					Options: map[string]string{
						"output-json": "if set sends JSON to stdout instead of a rendered table",
						"quiet":       "if set suppresses printing of progress messages to stderr",
					},
				},
			},
			{
				Name:     "report-buildpacks",
				HelpText: "Report all buildpacks used in installation",
				UsageDetails: plugin.Usage{
					Usage: "cf report-buildpacks",
					Options: map[string]string{
						"output-json": "if set sends JSON to stdout instead of a rendered table",
						"quiet":       "if set suppresses printing of progress messages to stderr",
					},
				},
			},
		},
	}
}

func main() {
	plugin.Start(&reportUsers{})
}
