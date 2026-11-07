package cli

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/Nextron-Labs/nextron-relic/internal/catalog"
	"github.com/Nextron-Labs/nextron-relic/pkg/config"
	"github.com/Nextron-Labs/nextron-relic/pkg/result"
)

func newValidateCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		asJSON bool
		goos   string
	)
	cmd := &cobra.Command{
		Use:   "validate <config.yaml>",
		Short: "Validate a configuration file without collecting",
		Long: `Validate a configuration file: YAML syntax, unknown fields, value ranges,
path pattern syntax, profile and artifact names. Each finding has a YAML path
and line/column. Exit code 0 if valid (warnings allowed), 4 if invalid.

With --os the artifact selection is evaluated for that platform, so a
configuration can be validated for Windows endpoints on any system.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if goos == "" {
				goos = runtime.GOOS
			}
			goos = config.NormalizeOS(goos)
			if !contains(config.ValidOS, goos) {
				return usageErr("--os: unknown operating system %q (windows, linux, darwin)", goos)
			}
			doc, issues := config.LoadFile(args[0])
			var selected []string
			if doc != nil && !issues.HasErrors() {
				cat, err := catalog.Builtin()
				if err != nil {
					return &exitError{code: result.ExitFailed, err: err}
				}
				sel, sis := cat.Select(doc.Config.Collect, goos)
				for _, is := range sis {
					doc.Locate(&is)
					issues = append(issues, is)
				}
				for _, a := range sel.Artifacts {
					selected = append(selected, a.Artifact.Name)
				}
				for _, n := range sel.NotApplicable {
					issues = append(issues, config.Issue{Severity: config.SeverityWarning, Path: "collect.artifacts",
						Message: fmt.Sprintf("artifact %q has no source for %s", n, goos)})
				}
			}
			valid := !issues.HasErrors()
			if asJSON {
				out := struct {
					Valid     bool          `json:"valid"`
					File      string        `json:"file"`
					SHA256    string        `json:"sha256,omitempty"`
					OS        string        `json:"os"`
					Issues    config.Issues `json:"issues"`
					Artifacts []string      `json:"artifacts"`
				}{Valid: valid, File: args[0], OS: goos, Issues: issues, Artifacts: selected}
				if out.Issues == nil {
					out.Issues = config.Issues{}
				}
				if out.Artifacts == nil {
					out.Artifacts = []string{}
				}
				if doc != nil {
					out.SHA256 = doc.SHA256
				}
				if err := writeJSON(stdout, out); err != nil {
					return err
				}
			} else {
				printIssues(stderr, issues)
				warn := len(issues) - len(issues.Errors())
				if valid {
					fmt.Fprintf(stdout, "%s: valid for %s (%d artifacts selected, %d warnings)\n", args[0], goos, len(selected), warn)
				} else {
					fmt.Fprintf(stdout, "%s: INVALID (%d errors, %d warnings)\n", args[0], len(issues.Errors()), warn)
				}
			}
			if !valid {
				return silentExit(result.ExitUsage)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	cmd.Flags().StringVar(&goos, "os", "", "target platform for artifact selection (windows, linux, darwin; default: this system)")
	return cmd
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func artifactOS(a *config.Artifact) []string {
	set := map[string]bool{}
	for _, s := range a.Sources {
		if len(s.OS) == 0 {
			for _, o := range config.ValidOS {
				set[o] = true
			}
		}
		for _, o := range s.OS {
			set[config.NormalizeOS(o)] = true
		}
	}
	var out []string
	for _, o := range config.ValidOS {
		if set[o] {
			out = append(out, o)
		}
	}
	return out
}

func profileMembership(cat *catalog.Catalog) map[string][]string {
	m := map[string][]string{}
	for _, p := range cat.ProfileNames() {
		names, _ := cat.ProfileArtifacts(p)
		for _, n := range names {
			m[n] = append(m[n], p)
		}
	}
	return m
}

func newListCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List built-in artifacts or profiles",
	}
	var (
		asJSON   bool
		goos     string
		profile  string
		search   string
		category string
	)
	arts := &cobra.Command{
		Use:   "artifacts",
		Short: "List built-in artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := catalog.Builtin()
			if err != nil {
				return err
			}
			goos = config.NormalizeOS(goos)
			if goos != "" && goos != "all" && !contains(config.ValidOS, goos) {
				return usageErr("--os: unknown operating system %q (windows, linux, darwin, all)", goos)
			}
			var filter map[string]bool
			if profile != "" {
				names, err := cat.ProfileArtifacts(profile)
				if err != nil {
					return usageErr("%v (available: %s)", err, strings.Join(cat.ProfileNames(), ", "))
				}
				filter = map[string]bool{}
				for _, n := range names {
					filter[n] = true
				}
			}
			member := profileMembership(cat)
			type row struct {
				Name        string   `json:"name"`
				Category    string   `json:"category"`
				OS          []string `json:"os"`
				Profiles    []string `json:"profiles"`
				Description string   `json:"description"`
			}
			rows := []row{}
			terms := strings.Fields(strings.ToLower(search))
			for _, a := range cat.Artifacts {
				if filter != nil && !filter[a.Name] {
					continue
				}
				if goos != "" && goos != "all" && !a.AppliesTo(goos) {
					continue
				}
				if category != "" && !strings.EqualFold(a.Category, category) {
					continue
				}
				if !matchesSearch(a, terms) {
					continue
				}
				rows = append(rows, row{a.Name, a.Category, artifactOS(a), member[a.Name], a.Description})
			}
			if asJSON {
				return writeJSON(stdout, rows)
			}
			tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tOS\tPROFILES\tDESCRIPTION")
			for _, r := range rows {
				desc := r.Description
				if len(desc) > 90 {
					desc = desc[:87] + "..."
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Name, strings.Join(r.OS, ","), strings.Join(r.Profiles, ","), desc)
			}
			return tw.Flush()
		},
	}
	arts.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	arts.Flags().StringVar(&goos, "os", "", "only artifacts for this platform (windows, linux, darwin)")
	arts.Flags().StringVar(&profile, "profile", "", "only artifacts of this profile")
	arts.Flags().StringVarP(&search, "search", "s", "", "only artifacts whose name, category, description or paths contain all of these words")
	arts.Flags().StringVar(&category, "category", "", "only artifacts of this category (e.g. persistence, execution, logs)")

	var profJSON bool
	profs := &cobra.Command{
		Use:   "profiles",
		Short: "List built-in profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := catalog.Builtin()
			if err != nil {
				return err
			}
			type row struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Artifacts   map[string]int `json:"artifacts_per_os"`
			}
			rows := []row{}
			for _, p := range cat.Profiles {
				counts := map[string]int{}
				for _, goos := range config.ValidOS {
					sel, _ := cat.Select(config.Collect{Profile: p.Name}, goos)
					counts[goos] = len(sel.Artifacts)
				}
				rows = append(rows, row{p.Name, p.Description, counts})
			}
			if profJSON {
				return writeJSON(stdout, rows)
			}
			tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tWINDOWS\tLINUX\tMACOS\tDESCRIPTION")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%s\n", r.Name, r.Artifacts["windows"], r.Artifacts["linux"], r.Artifacts["darwin"], r.Description)
			}
			return tw.Flush()
		},
	}
	profs.Flags().BoolVar(&profJSON, "json", false, "print as JSON")
	cmd.AddCommand(arts, profs)
	return cmd
}

// matchesSearch reports whether every term occurs in the artifact's name,
// category, description or path patterns (case-insensitive).
func matchesSearch(a *config.Artifact, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	var b strings.Builder
	b.WriteString(a.Name + "\n" + a.Category + "\n" + a.Description)
	for _, s := range a.Sources {
		for _, p := range s.Paths {
			b.WriteString("\n" + p)
		}
	}
	text := strings.ToLower(b.String())
	for _, t := range terms {
		if !strings.Contains(text, t) {
			return false
		}
	}
	return true
}

func newShowCmd(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the definition of an artifact or profile",
	}
	var asJSON bool
	art := &cobra.Command{
		Use:   "artifact <name>",
		Short: "Show an artifact definition (YAML, usable as template for custom_artifacts)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := catalog.Builtin()
			if err != nil {
				return err
			}
			a, ok := cat.Artifact(args[0])
			if !ok {
				return usageErr("unknown artifact %q; run 'relic list artifacts'", args[0])
			}
			if asJSON {
				return writeJSON(stdout, a)
			}
			return writeYAML(stdout, []*config.Artifact{a})
		},
	}
	art.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	var profJSON bool
	prof := &cobra.Command{
		Use:   "profile <name>",
		Short: "Show a profile and its resolved artifacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := catalog.Builtin()
			if err != nil {
				return err
			}
			p, ok := cat.Profile(args[0])
			if !ok {
				return usageErr("unknown profile %q (available: %s)", args[0], strings.Join(cat.ProfileNames(), ", "))
			}
			resolved := map[string][]string{}
			for _, goos := range config.ValidOS {
				sel, _ := cat.Select(config.Collect{Profile: p.Name}, goos)
				for _, a := range sel.Artifacts {
					resolved[goos] = append(resolved[goos], a.Artifact.Name)
				}
			}
			out := struct {
				*catalog.Profile `yaml:",inline"`
				Resolved         map[string][]string `yaml:"resolved" json:"resolved"`
			}{p, resolved}
			if profJSON {
				return writeJSON(stdout, out)
			}
			return writeYAML(stdout, out)
		},
	}
	prof.Flags().BoolVar(&profJSON, "json", false, "print as JSON")
	cmd.AddCommand(art, prof)
	return cmd
}

func writeYAML(w io.Writer, v interface{}) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return err
	}
	return enc.Close()
}

func newConfigCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect configurations",
	}
	var (
		path    string
		profile string
		asJSON  bool
	)
	show := &cobra.Command{
		Use:   "show",
		Short: "Print the effective configuration with all defaults (a good starting template)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var c *config.Config
			if path != "" {
				doc, issues := config.LoadFile(path)
				if issues.HasErrors() {
					printIssues(stderr, issues)
					return silentExit(result.ExitUsage)
				}
				c = doc.Config
			} else {
				c = config.Default()
				c.Collect.Profile = catalog.DefaultProfile
			}
			if profile != "" {
				c.Collect.Profile = profile
			}
			if asJSON {
				return writeJSON(stdout, c)
			}
			data, err := c.Marshal()
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "# Relic configuration (schema version %d). Sizes: B, KiB, MiB, GiB (or bytes);\n# durations: 30m, 2h (or seconds); 0 disables a limit.\n", config.CurrentVersion)
			_, err = stdout.Write(data)
			return err
		},
	}
	show.Flags().StringVarP(&path, "config", "c", "", "configuration file (default: built-in defaults)")
	show.Flags().StringVarP(&profile, "profile", "p", "", "set collect.profile")
	show.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	schema := &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema of the configuration format (for generators such as the Management Center)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := stdout.Write(config.JSONSchema)
			return err
		},
	}
	cmd.AddCommand(show, schema)
	return cmd
}
