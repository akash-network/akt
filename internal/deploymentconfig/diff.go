package deploymentconfig

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
)

// HasChanges reports whether a patch writes configuration or a deployment name.
// A secret-only rotation has no changes here; its secrets are a separate input.
func (patch Patch) HasChanges() bool {
	if patch.Name != nil {
		return true
	}
	for _, service := range patch.Services {
		if serviceAssigns(service) {
			return true
		}
	}
	return false
}

// Diff derives only supported configuration edits, then proves the result by
// applying them and comparing the entire requested document. Resource, routing,
// and other unrepresentable changes therefore cannot disappear silently.
func Diff(currentSDL, requestedSDL string) (Patch, error) {
	current, err := parseObject([]byte(currentSDL))
	if err != nil {
		return Patch{}, errors.New("cannot parse current SDL")
	}
	requested, err := parseObject([]byte(requestedSDL))
	if err != nil {
		return Patch{}, errors.New("cannot parse requested SDL")
	}
	oldServices, oldOK := current["services"].(map[string]any)
	newServices, newOK := requested["services"].(map[string]any)
	if !oldOK || !newOK {
		return Patch{}, errors.New("SDL must declare services")
	}
	patch := Patch{Services: make(map[string]ServicePatch)}
	for name, raw := range newServices {
		desired, ok := raw.(map[string]any)
		old, exists := oldServices[name].(map[string]any)
		if !ok || !exists {
			return Patch{}, unrepresentable()
		}
		fields := make(map[string]any)
		for _, field := range []string{"image", "command", "args", "credentials"} {
			if reflect.DeepEqual(old[field], desired[field]) {
				continue
			}
			if field == "image" && desired[field] == nil {
				return Patch{}, unrepresentable()
			}
			fields[field] = desired[field]
			if field == "credentials" && desired[field] != nil {
				credentials, ok := desired[field].(map[string]any)
				if !ok {
					return Patch{}, unrepresentable()
				}
				fields[field] = map[string]any{
					"host": credentials["host"], "username": credentials["username"], "password": credentials["password"],
				}
			}
		}
		env, err := diffEnv(old["env"], desired["env"])
		if err != nil {
			return Patch{}, err
		}
		if len(env) > 0 {
			fields["env"] = env
		}
		expose, err := diffExpose(old["expose"], desired["expose"])
		if err != nil {
			return Patch{}, err
		}
		if len(expose) > 0 {
			fields["expose"] = expose
		}
		storage, err := diffStorage(old, desired)
		if err != nil {
			return Patch{}, err
		}
		if len(storage) > 0 {
			fields["storage"] = storage
		}
		if len(fields) == 0 {
			continue
		}
		encoded, err := json.Marshal(fields)
		if err != nil {
			return Patch{}, unrepresentable()
		}
		var service ServicePatch
		if err := decodeStrict(encoded, &service); err != nil {
			return Patch{}, unrepresentable()
		}
		patch.Services[name] = service
	}
	if err := Validate(patch); err != nil {
		return Patch{}, err
	}
	result, err := Apply(currentSDL, patch)
	if err != nil {
		return Patch{}, err
	}
	applied, err := parseObject([]byte(result))
	if err != nil {
		return Patch{}, errors.New("cannot verify updated SDL")
	}
	normalizeSDL(applied)
	normalizeSDL(requested)
	if !reflect.DeepEqual(applied, requested) {
		return Patch{}, unrepresentable()
	}
	return patch, nil
}

func unrepresentable() error {
	return errors.New("requested SDL changes cannot be represented by a configuration update; use redeploy")
}

func diffEnv(current, requested any) (map[string]any, error) {
	if reflect.DeepEqual(current, requested) {
		return nil, nil
	}
	old, err := envValues(current)
	if err != nil {
		return nil, err
	}
	desired, err := envValues(requested)
	if err != nil {
		return nil, err
	}
	patch := make(map[string]any)
	for name := range old {
		if _, exists := desired[name]; !exists {
			patch[name] = nil
		}
	}
	for name, value := range desired {
		if previous, exists := old[name]; exists && reflect.DeepEqual(previous, value) {
			continue
		}
		if value == nil {
			return nil, unrepresentable()
		}
		patch[name] = value
	}
	return patch, nil
}

func envValues(raw any) (map[string]any, error) {
	result := make(map[string]any)
	if raw == nil {
		return result, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, unrepresentable()
	}
	for _, value := range values {
		entry, ok := value.(string)
		if !ok {
			return nil, unrepresentable()
		}
		key, content, hasValue := splitEnv(entry)
		if _, duplicate := result[key]; duplicate {
			return nil, errors.New("SDL contains duplicate environment variables; remove duplicates before updating")
		}
		if hasValue {
			result[key] = content
		} else {
			result[key] = nil
		}
	}
	return result, nil
}

func splitEnv(entry string) (string, string, bool) {
	for i := range entry {
		if entry[i] == '=' {
			return entry[:i], entry[i+1:], true
		}
	}
	return entry, "", false
}

func diffExpose(current, requested any) (map[string]any, error) {
	if reflect.DeepEqual(current, requested) {
		return nil, nil
	}
	old, okOld := current.([]any)
	desired, okNew := requested.([]any)
	if !okOld || !okNew || len(old) != len(desired) {
		return nil, unrepresentable()
	}
	patch := make(map[string]any)
	for index, raw := range desired {
		next, nextOK := raw.(map[string]any)
		prior, priorOK := old[index].(map[string]any)
		if !nextOK || !priorOK {
			return nil, unrepresentable()
		}
		fields := make(map[string]any)
		for _, field := range []string{"port", "as", "accept"} {
			if !reflect.DeepEqual(prior[field], next[field]) {
				fields[field] = next[field]
			}
		}
		oldOptions, _ := prior["http_options"].(map[string]any)
		newOptions, _ := next["http_options"].(map[string]any)
		options := make(map[string]any)
		for _, field := range []struct{ sdl, patch string }{
			{"max_body_size", "maxBodySize"}, {"read_timeout", "readTimeout"}, {"send_timeout", "sendTimeout"},
			{"next_tries", "nextTries"}, {"next_timeout", "nextTimeout"}, {"next_cases", "nextCases"},
		} {
			if !reflect.DeepEqual(oldOptions[field.sdl], newOptions[field.sdl]) {
				options[field.patch] = newOptions[field.sdl]
			}
		}
		if len(options) > 0 {
			fields["httpOptions"] = options
		}
		if len(fields) == 0 {
			continue
		}
		port, ok := prior["port"].(float64)
		if !ok {
			return nil, unrepresentable()
		}
		key := strconv.FormatFloat(port, 'f', -1, 64)
		if _, exists := patch[key]; exists {
			return nil, unrepresentable()
		}
		patch[key] = fields
	}
	return patch, nil
}

func diffStorage(current, requested map[string]any) (map[string]any, error) {
	oldParams, _ := current["params"].(map[string]any)
	newParams, _ := requested["params"].(map[string]any)
	oldStorage, _ := oldParams["storage"].(map[string]any)
	newStorage, _ := newParams["storage"].(map[string]any)
	patch := make(map[string]any)
	for name, raw := range newStorage {
		desired, ok := raw.(map[string]any)
		prior, exists := oldStorage[name].(map[string]any)
		if !ok || !exists {
			return nil, unrepresentable()
		}
		fields := make(map[string]any)
		for _, field := range []string{"mount", "readOnly"} {
			if !reflect.DeepEqual(prior[field], desired[field]) {
				fields[field] = desired[field]
			}
		}
		if len(fields) > 0 {
			patch[name] = fields
		}
	}
	return patch, nil
}

// Environment declaration order does not affect the manifest. Empty env and
// null clearable fields have the same meaning as absent fields.
func normalizeSDL(document map[string]any) {
	services, _ := document["services"].(map[string]any)
	for _, raw := range services {
		service, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range []string{"command", "args", "credentials", "env"} {
			if service[field] == nil {
				delete(service, field)
			}
		}
		env, ok := service["env"].([]any)
		if !ok {
			continue
		}
		if len(env) == 0 {
			delete(service, "env")
			continue
		}
		sort.SliceStable(env, func(i, j int) bool {
			a, aOK := env[i].(string)
			b, bOK := env[j].(string)
			return aOK && bOK && a < b
		})
	}
}
