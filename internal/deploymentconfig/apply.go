package deploymentconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Apply changes only explicitly named mutable service fields. Deployment
// names and concurrency versions belong to Console metadata, not the SDL.
func Apply(sdl string, patch Patch) (string, error) {
	if err := Validate(patch); err != nil {
		return "", err
	}
	document, err := parseObject([]byte(sdl))
	if err != nil {
		return "", fmt.Errorf("invalid SDL: %w", err)
	}
	if err := rejectSharedServices([]byte(sdl), patch); err != nil {
		return "", err
	}
	services, ok := document["services"].(map[string]any)
	if !ok {
		return "", errors.New("SDL must declare services")
	}
	for name, change := range patch.Services {
		service, ok := services[name].(map[string]any)
		if !ok {
			return "", errors.New("patch names a service not declared in the SDL")
		}
		if change.Image != nil {
			service["image"] = *change.Image
		}
		for _, field := range []struct {
			name string
			raw  json.RawMessage
		}{{"command", change.Command}, {"args", change.Args}, {"credentials", change.Credentials}} {
			if len(field.raw) == 0 {
				continue
			}
			var value any
			_ = json.Unmarshal(field.raw, &value) // Validate already checked each clearable field.
			if value == nil {
				delete(service, field.name)
			} else if field.name == "credentials" {
				credentials, ok := service["credentials"].(map[string]any)
				if !ok {
					credentials = make(map[string]any)
				}
				for key, value := range value.(map[string]any) {
					credentials[key] = value
				}
				service[field.name] = credentials
			} else {
				service[field.name] = value
			}
		}
		if len(change.Env) > 0 {
			applyEnv(service, change.Env)
		}
		if err := applyExpose(service, change.Expose); err != nil {
			return "", err
		}
		if err := applyStorage(service, change.Storage); err != nil {
			return "", err
		}
	}
	encoded, _ := yaml.Marshal(document) // The parsed tree and typed patch contain only YAML values.
	if len(encoded) > maxDocumentBytes {
		return "", errors.New("updated SDL exceeds 1 MiB")
	}
	return string(encoded), nil
}

func applyEnv(service map[string]any, patch map[string]*string) {
	source, _ := service["env"].([]any)
	values := make([]any, 0, len(source)+len(patch))
	for _, entry := range source {
		value, ok := entry.(string)
		if !ok {
			values = append(values, entry)
			continue
		}
		key, _, _ := strings.Cut(value, "=")
		if _, changed := patch[key]; !changed {
			values = append(values, entry)
		}
	}
	keys := make([]string, 0, len(patch))
	for key := range patch {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if value := patch[key]; value != nil {
			values = append(values, key+"="+*value)
		}
	}
	if len(values) == 0 {
		delete(service, "env")
	} else {
		service["env"] = values
	}
}

func applyExpose(service map[string]any, patch map[string]ExposePatch) error {
	if len(patch) == 0 {
		return nil
	}
	exposed, _ := service["expose"].([]any)
	before := make([]map[string]any, len(exposed))
	after := make([]map[string]any, len(exposed))
	changed := make(map[int]ExposePatch)
	for i, raw := range exposed {
		entry, ok := raw.(map[string]any)
		if !ok {
			return errors.New("SDL expose entries must be objects")
		}
		before[i], after[i] = entry, copyObject(entry)
	}
	for port, entryPatch := range patch {
		index := -1
		for i, entry := range before {
			number, ok := entry["port"].(float64)
			if !ok || strconv.FormatFloat(number, 'f', -1, 64) != port {
				continue
			}
			if index != -1 {
				return errors.New("patch port matches multiple endpoints; use redeploy to change ambiguous endpoints")
			}
			index = i
		}
		if index == -1 {
			return errors.New("patch names a port not declared in the SDL")
		}
		changed[index] = entryPatch
		if entryPatch.Port != nil {
			after[index]["port"] = float64(*entryPatch.Port)
		}
		if entryPatch.As != nil {
			after[index]["as"] = float64(*entryPatch.As)
		}
		if endpointKind(before[index]) != endpointKind(after[index]) {
			return errors.New("patch changes an endpoint kind; use redeploy")
		}
		if hasLeasedIP(before[index]) && (before[index]["port"] != after[index]["port"] || externalPort(before[index]) != externalPort(after[index])) {
			return errors.New("patch moves a port reached through a leased IP; use redeploy")
		}
	}
	for i, change := range changed {
		for j := range after {
			if i == j {
				continue
			}
			if change.Port != nil && after[i]["port"] == after[j]["port"] {
				return errors.New("patch creates a container port collision")
			}
			if externalPort(before[i]) != externalPort(after[i]) && externalPort(after[i]) == externalPort(after[j]) {
				return errors.New("patch creates an external port collision")
			}
		}
	}
	for index, change := range changed {
		entry := after[index]
		if change.Accept != nil {
			entry["accept"] = *change.Accept
		}
		if change.HTTPOptions != nil {
			fields := httpOptionValues(change.HTTPOptions)
			if len(fields) > 0 {
				options, ok := entry["http_options"].(map[string]any)
				if !ok {
					options = make(map[string]any)
				}
				for name, value := range fields {
					options[name] = value
				}
				entry["http_options"] = options
			}
		}
		exposed[index] = entry
	}
	return nil
}

func httpOptionValues(options *HTTPOptionsPatch) map[string]any {
	fields := make(map[string]any)
	for _, field := range []struct {
		name  string
		value *uint32
	}{
		{"max_body_size", options.MaxBodySize}, {"read_timeout", options.ReadTimeout},
		{"send_timeout", options.SendTimeout}, {"next_tries", options.NextTries}, {"next_timeout", options.NextTimeout},
	} {
		if field.value != nil {
			fields[field.name] = float64(*field.value)
		}
	}
	if options.NextCases != nil {
		fields["next_cases"] = *options.NextCases
	}
	return fields
}

func externalPort(entry map[string]any) float64 {
	if as, ok := entry["as"].(float64); ok && as != 0 {
		return as
	}
	port, _ := entry["port"].(float64)
	return port
}

func endpointKind(entry map[string]any) string {
	targets, _ := entry["to"].([]any)
	for _, value := range targets {
		target, _ := value.(map[string]any)
		if global, _ := target["global"].(bool); !global {
			continue
		}
		protocol, _ := entry["proto"].(string)
		if (protocol == "" || strings.EqualFold(protocol, "tcp")) && externalPort(entry) == 80 {
			return "http"
		}
		return "random"
	}
	return "internal"
}

func hasLeasedIP(entry map[string]any) bool {
	targets, _ := entry["to"].([]any)
	for _, value := range targets {
		target, _ := value.(map[string]any)
		if ip, _ := target["ip"].(string); ip != "" {
			return true
		}
	}
	return false
}

func applyStorage(service map[string]any, patch map[string]StoragePatch) error {
	params, _ := service["params"].(map[string]any)
	storage, _ := params["storage"].(map[string]any)
	for name, change := range patch {
		volume, ok := storage[name].(map[string]any)
		if !ok {
			return errors.New("patch names a storage volume not declared in the SDL")
		}
		if change.Mount != nil {
			volume["mount"] = *change.Mount
		}
		if change.ReadOnly != nil {
			volume["readOnly"] = *change.ReadOnly
		}
	}
	return nil
}

func copyObject(source map[string]any) map[string]any {
	target := make(map[string]any, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}

// Mutating an aliased service may affect other SDL locations on the server.
// Refuse that ambiguity while permitting anchors in resource declarations.
func rejectSharedServices(data []byte, patch Patch) error {
	var document yaml.Node
	_ = yaml.Unmarshal(data, &document) // Apply already parsed this exact document.
	referenced := make(map[*yaml.Node]bool)
	var markShared func(*yaml.Node)
	markShared = func(node *yaml.Node) {
		if referenced[node] {
			return
		}
		referenced[node] = true
		for _, child := range node.Content {
			markShared(child)
		}
	}
	var scan func(*yaml.Node)
	scan = func(node *yaml.Node) {
		if node.Kind == yaml.AliasNode {
			markShared(node.Alias)
		}
		for _, child := range node.Content {
			scan(child)
		}
	}
	scan(&document)
	root := document.Content[0]
	services := mappingNode(root, "services")
	for _, change := range patch.Services {
		if serviceAssigns(change) && (services == nil && mappingNode(root, "<<") != nil || services != nil && (services.Kind == yaml.AliasNode || referenced[services])) {
			return errors.New("SDL anchors share patched service definitions; expand its anchors before updating")
		}
	}
	if services == nil {
		return nil
	}
	for name, change := range patch.Services {
		if !serviceAssigns(change) {
			continue
		}
		service := mappingNode(services, name)
		if service == nil {
			continue
		}
		shared := func(node *yaml.Node) bool {
			return node != nil && (node.Kind == yaml.AliasNode || referenced[node])
		}
		if shared(service) || mappingNode(service, "<<") != nil {
			return errors.New("SDL anchors share a patched service definition; expand its anchors before updating")
		}
		if len(change.Env) > 0 && shared(mappingNode(service, "env")) || len(change.Credentials) > 0 && shared(mappingNode(service, "credentials")) {
			return errors.New("SDL anchors share patched environment or credentials; expand its anchors before updating")
		}
		exposed := mappingNode(service, "expose")
		if exposed != nil && len(change.Expose) > 0 {
			if shared(exposed) {
				return errors.New("SDL anchors share patched endpoints; expand its anchors before updating")
			}
			for _, entry := range exposed.Content {
				target := entry
				if target.Kind == yaml.AliasNode {
					target = target.Alias
				}
				port := mappingNode(target, "port")
				if port != nil && port.Kind == yaml.AliasNode {
					port = port.Alias
				}
				if port == nil {
					continue
				}
				entryPatch, assigned := change.Expose[port.Value]
				if !assigned {
					continue
				}
				if shared(entry) || entryPatch.HTTPOptions != nil && len(httpOptionValues(entryPatch.HTTPOptions)) > 0 && shared(mappingNode(entry, "http_options")) {
					return errors.New("SDL anchors share a patched endpoint; expand its anchors before updating")
				}
			}
		}
		params := mappingNode(service, "params")
		if params != nil && len(change.Storage) > 0 {
			storage := mappingNode(params, "storage")
			if shared(params) || shared(storage) {
				return errors.New("SDL anchors share patched storage; expand its anchors before updating")
			}
			if storage != nil {
				for volume, volumePatch := range change.Storage {
					if (volumePatch.Mount != nil || volumePatch.ReadOnly != nil) && shared(mappingNode(storage, volume)) {
						return errors.New("SDL anchors share a patched volume; expand its anchors before updating")
					}
				}
			}
		}
	}
	return nil
}

func mappingNode(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func serviceAssigns(patch ServicePatch) bool {
	if patch.Image != nil || len(patch.Command) > 0 || len(patch.Args) > 0 || len(patch.Credentials) > 0 || len(patch.Env) > 0 {
		return true
	}
	for _, expose := range patch.Expose {
		if expose.Port != nil || expose.As != nil || expose.Accept != nil || expose.HTTPOptions != nil && len(httpOptionValues(expose.HTTPOptions)) > 0 {
			return true
		}
	}
	for _, storage := range patch.Storage {
		if storage.Mount != nil || storage.ReadOnly != nil {
			return true
		}
	}
	return false
}
