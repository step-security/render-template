package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"time"
	_ "time/tzdata"

	"github.com/caarlos0/env/v10"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"
)

type vars map[string]interface{}

type config struct {
	Template   string `env:"INPUT_TEMPLATE" envDefault:".kube.yml"`
	Vars       vars   `env:"INPUT_VARS" envDefault:""`
	VarsPath   string `env:"INPUT_VARS_PATH" envDefault:""`
	ResultPath string `env:"INPUT_RESULT_PATH" envDefault:""`
}

func main() {
	if err := run(); err != nil {
		fmt.Printf("::error::%v", err)
		os.Exit(1)
	}
}

func validateSubscription() {
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	var repoPrivate *bool

	if eventPath != "" {
		if eventData, err := os.ReadFile(eventPath); err == nil {
			var event struct {
				Repository struct {
					Private *bool `json:"private"`
				} `json:"repository"`
			}
			if err := json.Unmarshal(eventData, &event); err == nil {
				repoPrivate = event.Repository.Private
			}
		}
	}

	upstream := "chuhlomin/render-template"
	action := os.Getenv("GITHUB_ACTION_REPOSITORY")
	docsURL := "https://docs.stepsecurity.io/actions/stepsecurity-maintained-actions"

	fmt.Println()
	fmt.Println("\x1b[1;36mStepSecurity Maintained Action\x1b[0m")
	fmt.Printf("Secure drop-in replacement for %s\n", upstream)
	if repoPrivate != nil && *repoPrivate == false {
		fmt.Println("\x1b[32m\u2713 Free for public repositories\x1b[0m")
	}
	fmt.Printf("\x1b[36mLearn more:\x1b[0m %s\n", docsURL)
	fmt.Println()

	if repoPrivate != nil && *repoPrivate == false {
		return
	}

	serverURL := os.Getenv("GITHUB_SERVER_URL")
	if serverURL == "" {
		serverURL = "https://github.com"
	}

	body := map[string]string{"action": action}
	if serverURL != "https://github.com" {
		body["ghes_server"] = serverURL
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		fmt.Println("Timeout or API not reachable. Continuing to next step.")
		return
	}

	apiURL := fmt.Sprintf("https://agent.api.stepsecurity.io/v1/github/%s/actions/maintained-actions-subscription", os.Getenv("GITHUB_REPOSITORY"))

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Println("Timeout or API not reachable. Continuing to next step.")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		fmt.Printf("::error::\x1b[1;31mThis action requires a StepSecurity subscription for private repositories.\x1b[0m\n")
		fmt.Printf("::error::\x1b[31mLearn how to enable a subscription: %s\x1b[0m\n", docsURL)
		os.Exit(1)
	}
}

func run() error {
	validateSubscription()

	var c config
	parsers := map[reflect.Type]env.ParserFunc{
		reflect.TypeOf(vars{}): varsParser,
	}
	if err := env.ParseWithOptions(&c, env.Options{FuncMap: parsers}); err != nil {
		return err
	}

	if c.VarsPath != "" {
		varsFile, err := os.ReadFile(c.VarsPath)
		if err != nil {
			return fmt.Errorf("failed to read vars file %q: %w", c.VarsPath, err)
		}
		var varsFromFile vars
		if err = yaml.Unmarshal(varsFile, &varsFromFile); err != nil {
			return fmt.Errorf("failed to parse vars file %q: %w", c.VarsPath, err)
		}
		c.Vars = mergeVars(c.Vars, varsFromFile)
	}

	output, err := renderTemplate(c.Template, c.Vars)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	if err := writeOutput(output); err != nil {
		return err
	}

	if c.ResultPath != "" {
		err := os.WriteFile(c.ResultPath, []byte(output), 0o644)
		if err != nil {
			return fmt.Errorf("failed to write file %q: %w", c.ResultPath, err)
		}
	}

	return nil
}

func varsParser(v string) (interface{}, error) {
	m := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(v), &m)
	if err != nil {
		return nil, fmt.Errorf("unable to parse Vars: %w", err)
	}
	return m, nil
}

func mergeVars(a, b vars) vars {
	if a == nil {
		return b
	}

	for k, v := range b {
		if _, ok := a[k]; ok {
			continue
		}
		a[k] = v
	}
	return a
}

var funcMap = template.FuncMap{
	"date": func(format string, in interface{}) string {
		var t time.Time
		switch v := in.(type) {
		case string:
			var err error
			t, err = time.Parse(time.RFC3339, v)
			if err != nil {
				log.Printf("failed to parse date %q: %v", v, err)
				return v
			}
		case time.Time:
			t = v
		default:
			log.Printf("unsupported type %T for date", in)
			return fmt.Sprintf("%v", in)
		}

		timezone := os.Getenv("INPUT_TIMEZONE")
		if timezone != "" {
			loc, err := time.LoadLocation(timezone)
			if err != nil {
				log.Printf("failed to load timezone %q: %v", timezone, err)
				return in.(string)
			}
			t = t.In(loc)
		}

		return t.Format(format)
	},
	"mdlink": func(text, url string) string {
		return fmt.Sprintf("[%s](%s)", text, url)
	},
	"number": func(in string) string {
		p := message.NewPrinter(language.English)
		d, err := strconv.ParseInt(in, 10, 64)
		if err != nil {
			log.Printf("failed to parse number %q: %v", in, err)
			return in
		}
		return p.Sprintf("%d", d)
	},
	"base64": func(in string) string {
		return base64.StdEncoding.EncodeToString([]byte(in))
	},
	"split": func(sep string, in string) []string {
		return strings.Split(in, sep)
	},
	"toJSON": func(in interface{}) string {
		b, err := json.Marshal(in)
		if err != nil {
			log.Printf("failed to marshal to JSON: %v", err)
			return ""
		}
		return string(b)
	},
}

func renderTemplate(templateFilePath string, vars vars) (string, error) {
	b, err := os.ReadFile(templateFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("template file not found (%q)", templateFilePath)
		}
		if errors.Is(err, os.ErrPermission) {
			return "", fmt.Errorf("have no permissions to read template file (%q)", templateFilePath)
		}
		return "", fmt.Errorf("failed to read template %q: %w", templateFilePath, err)
	}

	tmpl, err := template.
		New(templateFilePath).
		Option("missingkey=error").
		Funcs(funcMap).
		Parse(string(b))
	if err != nil {
		return "", err
	}

	var result bytes.Buffer
	if err := tmpl.Execute(&result, vars); err != nil {
		return "", err
	}

	return result.String(), nil
}

func writeOutput(output string) error {
	githubOutput := formatOutput("result", output)
	if githubOutput == "" {
		return nil
	}

	path := os.Getenv("GITHUB_OUTPUT")

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf(
			"failed to open result file %q: %v. "+
				"If you are using self-hosted runners "+
				"make sure they are updated to version 2.297.0 or greater",
			path,
			err,
		)
	}
	defer f.Close()

	if _, err = f.WriteString(githubOutput); err != nil {
		return fmt.Errorf("failed to write result to file %q: %w", path, err)
	}

	return nil
}

func formatOutput(name, value string) string {
	if value == "" {
		return ""
	}

	// if value contains new line, use multiline format
	if bytes.ContainsRune([]byte(value), '\n') {
		return fmt.Sprintf("%s<<OUTPUT\n%s\nOUTPUT", name, value)
	}

	return fmt.Sprintf("%s=%s", name, value)
}
