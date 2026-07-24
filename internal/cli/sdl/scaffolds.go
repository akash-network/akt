// Package sdl implements the transport-independent `akt sdl` command group:
// scaffold listing, SDL generation, and offline validation. Everything runs
// locally — no context, keyring, or RPC endpoint is required, so the group
// declares no capability requirements.
//
// The scaffold shapes are ported from console-axi's sdl/templates
// (registry.ts, common.ts, web.ts, gpu.ts, multi-service.ts, ip-lease.ts).
package sdl

import (
	"bytes"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Pricing defaults ported from console-axi's sdl/templates/common.ts and
// gpu.ts: generous per-block ceilings (in uact) so bids arrive.
const (
	defaultPrice    = 10000  // uact/block
	gpuDefaultPrice = 100000 // uact/block — GPU compute is priced higher
)

// Options are the generation parameters for `akt sdl init`. These are not
// positional-argument twins: every field is an optional knob with a
// per-scaffold default, so `akt sdl init <scaffold>` with no flags always
// produces a deployable SDL. Pointer fields distinguish "flag not set" from
// an explicit zero.
type Options struct {
	Name     string
	Image    string
	Port     *int
	As       *int
	CPU      string
	Memory   string
	Storage  string
	Count    *int
	Price    *int // max price per block, in uact
	Env      []string
	GPU      *int
	GPUModel string
}

// Scaffold is one built-in SDL shape `akt sdl init` can generate.
type Scaffold struct {
	Name        string
	Description string
	// Params lists the flags that meaningfully affect this scaffold,
	// shown by `akt sdl scaffolds`.
	Params []string
	// Build assembles the SDL document as a yaml.Node tree so that field
	// order is stable and follows SDL conventions (version, services,
	// profiles, deployment).
	Build func(o Options) *yaml.Node
}

// scaffoldRegistry is the ordered scaffold registry, ported from
// console-axi's sdl/templates/registry.ts. Add a scaffold by defining a new
// Scaffold value and appending it here — nothing else needs to change.
var scaffoldRegistry = []Scaffold{webScaffold, gpuScaffold, multiServiceScaffold, ipLeaseScaffold}

// Scaffolds returns the built-in scaffolds in registry order.
func Scaffolds() []Scaffold { return scaffoldRegistry }

// Lookup returns the scaffold with the given name, or nil if none exists.
func Lookup(name string) *Scaffold {
	for i := range scaffoldRegistry {
		if scaffoldRegistry[i].Name == name {
			return &scaffoldRegistry[i]
		}
	}

	return nil
}

// ScaffoldNames returns the registry names in order.
func ScaffoldNames() []string {
	names := make([]string, len(scaffoldRegistry))
	for i, s := range scaffoldRegistry {
		names[i] = s.Name
	}

	return names
}

// Marshal renders a scaffold document as YAML with 2-space indentation.
func Marshal(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		return nil, err
	}

	if err := enc.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// webScaffold ports console-axi's templates/web.ts: a single container with
// one HTTP port exposed to the internet.
var webScaffold = Scaffold{
	Name:        "web",
	Description: "Single web service with one HTTP port exposed to the internet.",
	Params:      []string{"--image", "--port", "--as", "--cpu", "--memory", "--storage", "--count", "--price", "--env"},
	Build: func(o Options) *yaml.Node {
		name := orStr(o.Name, "web")

		svc := mapNode(pair("image", strNode(orStr(o.Image, "nginx:1.27"))))
		if len(o.Env) > 0 {
			appendPair(svc, "env", envNode(o.Env))
		}
		appendPair(svc, "expose", exposeNode(orInt(o.Port, 80), orInt(o.As, 80),
			mapNode(pair("global", boolNode(true)))))

		return mapNode(
			pair("version", quotedNode("2.0")),
			pair("services", mapNode(pair(name, svc))),
			pair("profiles", mapNode(
				pair("compute", mapNode(pair(name, mapNode(
					pair("resources", computeResources(o, "0.5", "512Mi", "512Mi")))))),
				pair("placement", placementNode([]string{name}, orInt(o.Price, defaultPrice))),
			)),
			pair("deployment", deploymentNode(name, name, orInt(o.Count, 1))),
		)
	},
}

// gpuScaffold ports console-axi's templates/gpu.ts: a GPU workload
// (ML/inference) with an nvidia model requirement.
var gpuScaffold = Scaffold{
	Name:        "gpu",
	Description: "GPU workload (ML/inference) with an nvidia model requirement.",
	Params:      []string{"--image", "--gpu", "--gpu-model", "--port", "--as", "--cpu", "--memory", "--storage", "--count", "--price", "--env"},
	Build: func(o Options) *yaml.Node {
		name := orStr(o.Name, "app")

		svc := mapNode(pair("image", strNode(orStr(o.Image, "pytorch/pytorch:2.2.0-cuda12.1-cudnn8-runtime"))))
		if len(o.Env) > 0 {
			appendPair(svc, "env", envNode(o.Env))
		}
		appendPair(svc, "expose", exposeNode(orInt(o.Port, 8080), orInt(o.As, 80),
			mapNode(pair("global", boolNode(true)))))

		resources := computeResources(o, "4", "16Gi", "50Gi")
		appendPair(resources, "gpu", mapNode(
			pair("units", intNode(orInt(o.GPU, 1))),
			pair("attributes", mapNode(pair("vendor", mapNode(pair("nvidia",
				seqNode(mapNode(pair("model", strNode(orStr(o.GPUModel, "a100")))))))))),
		))

		return mapNode(
			pair("version", quotedNode("2.0")),
			pair("services", mapNode(pair(name, svc))),
			pair("profiles", mapNode(
				pair("compute", mapNode(pair(name, mapNode(pair("resources", resources))))),
				pair("placement", placementNode([]string{name}, orInt(o.Price, gpuDefaultPrice))),
			)),
			pair("deployment", deploymentNode(name, name, orInt(o.Count, 1))),
		)
	},
}

// multiServiceScaffold ports console-axi's templates/multi-service.ts: a
// public `app` plus a `db` with a persistent volume and internal-only
// networking. A starting point the user edits (images, env, sizes).
var multiServiceScaffold = Scaffold{
	Name:        "multi-service",
	Description: "App + database with a persistent volume and service-to-service networking.",
	Params:      []string{"--image", "--port", "--as", "--cpu", "--memory", "--storage", "--count", "--price", "--env"},
	Build: func(o Options) *yaml.Node {
		app := mapNode(
			pair("image", strNode(orStr(o.Image, "nginx:1.27"))),
			pair("dependencies", seqNode(mapNode(pair("service", strNode("db"))))),
		)
		if len(o.Env) > 0 {
			appendPair(app, "env", envNode(o.Env))
		}
		appendPair(app, "expose", exposeNode(orInt(o.Port, 80), orInt(o.As, 80),
			mapNode(pair("global", boolNode(true)))))

		db := mapNode(
			pair("image", strNode("postgres:16")),
			pair("env", envNode([]string{"POSTGRES_PASSWORD=changeme", "POSTGRES_USER=app", "POSTGRES_DB=app"})),
			pair("expose", seqNode(mapNode(
				pair("port", intNode(5432)),
				pair("to", seqNode(mapNode(pair("service", strNode("app"))))),
			))),
			pair("params", mapNode(pair("storage", mapNode(pair("db-data", mapNode(
				pair("mount", strNode("/var/lib/postgresql/data")),
				pair("readOnly", boolNode(false)),
			)))))),
		)

		dbResources := mapNode(
			pair("cpu", mapNode(pair("units", cpuUnitsNode("0.5")))),
			pair("memory", mapNode(pair("size", strNode("1Gi")))),
			pair("storage", seqNode(
				mapNode(pair("size", strNode("1Gi"))),
				mapNode(
					pair("name", strNode("db-data")),
					pair("size", strNode(orStr(o.Storage, "10Gi"))),
					pair("attributes", mapNode(
						pair("persistent", boolNode(true)),
						pair("class", strNode("beta2")),
					)),
				),
			)),
		)

		return mapNode(
			pair("version", quotedNode("2.0")),
			pair("services", mapNode(pair("app", app), pair("db", db))),
			pair("profiles", mapNode(
				pair("compute", mapNode(
					pair("app", mapNode(pair("resources", computeResources(o, "0.5", "512Mi", "512Mi")))),
					pair("db", mapNode(pair("resources", dbResources))),
				)),
				pair("placement", placementNode([]string{"app", "db"}, orInt(o.Price, defaultPrice))),
			)),
			pair("deployment", mapNode(
				pair("app", mapNode(pair("dcloud", mapNode(
					pair("profile", strNode("app")),
					pair("count", intNode(orInt(o.Count, 1))),
				)))),
				pair("db", mapNode(pair("dcloud", mapNode(
					pair("profile", strNode("db")),
					pair("count", intNode(1)),
				)))),
			)),
		)
	},
}

// ipLeaseScaffold ports console-axi's templates/ip-lease.ts: a service with
// a dedicated public IP (SDL v2.1 endpoints + expose to ip).
var ipLeaseScaffold = Scaffold{
	Name:        "ip-lease",
	Description: "Service with a dedicated public IP (endpoints + expose to ip).",
	Params:      []string{"--image", "--port", "--as", "--cpu", "--memory", "--storage", "--count", "--price", "--env"},
	Build: func(o Options) *yaml.Node {
		name := orStr(o.Name, "web")
		const endpoint = "appip"

		svc := mapNode(pair("image", strNode(orStr(o.Image, "nginx:1.27"))))
		if len(o.Env) > 0 {
			appendPair(svc, "env", envNode(o.Env))
		}
		appendPair(svc, "expose", exposeNode(orInt(o.Port, 80), orInt(o.As, 80), mapNode(
			pair("global", boolNode(true)),
			pair("ip", strNode(endpoint)),
		)))

		return mapNode(
			pair("version", quotedNode("2.1")),
			pair("endpoints", mapNode(pair(endpoint, mapNode(pair("kind", strNode("ip")))))),
			pair("services", mapNode(pair(name, svc))),
			pair("profiles", mapNode(
				pair("compute", mapNode(pair(name, mapNode(
					pair("resources", computeResources(o, "0.5", "512Mi", "512Mi")))))),
				pair("placement", placementNode([]string{name}, orInt(o.Price, defaultPrice))),
			)),
			pair("deployment", deploymentNode(name, name, orInt(o.Count, 1))),
		)
	},
}

// ---- shared SDL fragments (ported from templates/common.ts) --------------

// computeResources builds a simple (single ephemeral volume) resources block
// from the options plus per-scaffold defaults.
func computeResources(o Options, defCPU, defMemory, defStorage string) *yaml.Node {
	return mapNode(
		pair("cpu", mapNode(pair("units", cpuUnitsNode(orStr(o.CPU, defCPU))))),
		pair("memory", mapNode(pair("size", strNode(orStr(o.Memory, defMemory))))),
		pair("storage", mapNode(pair("size", strNode(orStr(o.Storage, defStorage))))),
	)
}

// placementNode prices every given compute profile at the same uact ceiling
// under a single "dcloud" placement group.
func placementNode(profiles []string, price int) *yaml.Node {
	pricing := make([]nodePair, len(profiles))
	for i, p := range profiles {
		pricing[i] = pair(p, mapNode(
			pair("denom", strNode("uact")),
			pair("amount", intNode(price)),
		))
	}

	return mapNode(pair("dcloud", mapNode(pair("pricing", mapNode(pricing...)))))
}

// deploymentNode maps one service onto one compute profile.
func deploymentNode(service, profile string, count int) *yaml.Node {
	return mapNode(pair(service, mapNode(pair("dcloud", mapNode(
		pair("profile", strNode(profile)),
		pair("count", intNode(count)),
	)))))
}

// exposeNode builds a single-entry expose list with the given `to` targets.
func exposeNode(port, as int, to ...*yaml.Node) *yaml.Node {
	return seqNode(mapNode(
		pair("port", intNode(port)),
		pair("as", intNode(as)),
		pair("to", seqNode(to...)),
	))
}

// ---- yaml.Node construction helpers ---------------------------------------

type nodePair struct {
	key string
	val *yaml.Node
}

func pair(key string, val *yaml.Node) nodePair { return nodePair{key: key, val: val} }

func mapNode(pairs ...nodePair) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	for _, p := range pairs {
		n.Content = append(n.Content, strNode(p.key), p.val)
	}

	return n
}

func appendPair(m *yaml.Node, key string, val *yaml.Node) {
	m.Content = append(m.Content, strNode(key), val)
}

func seqNode(items ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Content: items}
}

func strNode(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

// quotedNode forces double quotes so values like "2.0" stay strings.
func quotedNode(v string) *yaml.Node {
	n := strNode(v)
	n.Style = yaml.DoubleQuotedStyle

	return n
}

func intNode(v int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}
}

func boolNode(v bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v)}
}

var numericValueRe = regexp.MustCompile(`^\d+(\.\d+)?$`)

// cpuUnitsNode keeps numeric cpu strings numeric (0.5, 2) and passes
// millicore strings ("500m") through as strings, mirroring the reference's
// cpuUnits helper in templates/common.ts.
func cpuUnitsNode(v string) *yaml.Node {
	if numericValueRe.MatchString(v) {
		// Plain scalar: the YAML encoder resolves it to an int or float.
		return &yaml.Node{Kind: yaml.ScalarNode, Value: v}
	}

	return strNode(v)
}

func envNode(env []string) *yaml.Node {
	items := make([]*yaml.Node, len(env))
	for i, e := range env {
		items[i] = strNode(e)
	}

	return seqNode(items...)
}

func orStr(v, def string) string {
	if v != "" {
		return v
	}

	return def
}

func orInt(v *int, def int) int {
	if v != nil {
		return *v
	}

	return def
}
