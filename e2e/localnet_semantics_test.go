package e2e

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types/bech32"
)

type localnetSemanticDeployment struct {
	Owner string
	DSeq  uint64
	State string
}

type localnetSemanticOrder struct {
	Owner string
	DSeq  uint64
	GSeq  uint32
	OSeq  uint32
	State string
}

type localnetSemanticGroup struct {
	Owner string
	DSeq  uint64
	GSeq  uint32
	State string
}

type localnetSemanticEscrow struct {
	Owner  string
	DSeq   uint64
	State  string
	Denom  string
	Amount string
}

type localnetSemanticMutation struct {
	TxHash  string
	Code    uint32
	Status  string
	Account string
	Action  string
}

func localnetSemanticValidateSuccessfulJSON(stdout, stderr string, exitCode int) error {
	if exitCode != 0 {
		return fmt.Errorf("JSON command exited %d", exitCode)
	}
	if strings.TrimSpace(stderr) != "" {
		return errors.New("successful non-interactive JSON command wrote to stderr")
	}

	object, err := localnetSemanticJSONObject([]byte(stdout))
	if err != nil {
		return fmt.Errorf("decode command JSON: %w", err)
	}
	if len(object) == 0 {
		return errors.New("command returned an empty JSON object")
	}

	return nil
}

func localnetSemanticAssertDeploymentCollections(aktJSON, nativeJSON []byte, expected []localnetSemanticDeployment) error {
	want, err := localnetSemanticDeploymentSet(expected)
	if err != nil {
		return fmt.Errorf("invalid expected deployment set: %w", err)
	}

	for _, document := range []struct {
		name string
		data []byte
	}{
		{name: "akt", data: aktJSON},
		{name: "native", data: nativeJSON},
	} {
		deployments, err := localnetSemanticDecodeDeploymentCollection(document.data)
		if err != nil {
			return fmt.Errorf("%s deployment collection: %w", document.name, err)
		}
		got, err := localnetSemanticDeploymentSet(deployments)
		if err != nil {
			return fmt.Errorf("%s deployment collection: %w", document.name, err)
		}
		if err := localnetSemanticCompareDeploymentSets(got, want); err != nil {
			return fmt.Errorf("%s deployment collection: %w", document.name, err)
		}
	}

	return nil
}

func localnetSemanticDecodeDeploymentCollection(data []byte) ([]localnetSemanticDeployment, error) {
	root, err := localnetSemanticJSONObject(data)
	if err != nil {
		return nil, err
	}
	items, err := localnetSemanticJSONArrayField(root, "deployments")
	if err != nil {
		return nil, err
	}

	result := make([]localnetSemanticDeployment, 0, len(items))
	for index, item := range items {
		wrapper, err := localnetSemanticJSONObject(item)
		if err != nil {
			return nil, fmt.Errorf("deployment %d: %w", index, err)
		}
		deployment, err := localnetSemanticJSONObjectField(wrapper, "deployment")
		if err != nil {
			return nil, fmt.Errorf("deployment %d: %w", index, err)
		}
		identity, err := localnetSemanticJSONObjectField(deployment, "id")
		if err != nil {
			return nil, fmt.Errorf("deployment %d: %w", index, err)
		}
		owner, err := localnetSemanticRequiredString(identity, "owner")
		if err != nil {
			return nil, fmt.Errorf("deployment %d identity: %w", index, err)
		}
		if err := localnetSemanticValidateAddress(owner); err != nil {
			return nil, fmt.Errorf("deployment %d identity: %w", index, err)
		}
		dseq, err := localnetSemanticPositiveUint(identity, "dseq", 64)
		if err != nil {
			return nil, fmt.Errorf("deployment %d identity: %w", index, err)
		}
		state, err := localnetSemanticRequiredString(deployment, "state")
		if err != nil {
			return nil, fmt.Errorf("deployment %d: %w", index, err)
		}
		if !localnetSemanticStringIn(state, "active", "closed") {
			return nil, fmt.Errorf("deployment %d has invalid state %q", index, state)
		}
		result = append(result, localnetSemanticDeployment{Owner: owner, DSeq: dseq, State: state})
	}

	return result, nil
}

func localnetSemanticDeploymentSet(deployments []localnetSemanticDeployment) (map[string]string, error) {
	result := make(map[string]string, len(deployments))
	for index, deployment := range deployments {
		if err := localnetSemanticValidateAddress(deployment.Owner); err != nil {
			return nil, fmt.Errorf("deployment %d: %w", index, err)
		}
		if deployment.DSeq == 0 {
			return nil, fmt.Errorf("deployment %d has zero dseq", index)
		}
		if !localnetSemanticStringIn(deployment.State, "active", "closed") {
			return nil, fmt.Errorf("deployment %d has invalid state %q", index, deployment.State)
		}
		key := deployment.Owner + "/" + strconv.FormatUint(deployment.DSeq, 10)
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate deployment identity %s", key)
		}
		result[key] = deployment.State
	}
	return result, nil
}

func localnetSemanticCompareDeploymentSets(got, want map[string]string) error {
	if len(got) != len(want) {
		return fmt.Errorf("returned %d deployments, want %d", len(got), len(want))
	}
	for identity, expectedState := range want {
		state, exists := got[identity]
		if !exists {
			return fmt.Errorf("missing deployment %s", identity)
		}
		if state != expectedState {
			return fmt.Errorf("deployment %s state = %q, want %q", identity, state, expectedState)
		}
	}
	return nil
}

func localnetSemanticAssertOrder(data []byte, expected localnetSemanticOrder) error {
	root, err := localnetSemanticJSONObject(data)
	if err != nil {
		return err
	}

	orderRaw, hasOrder := root["order"]
	ordersRaw, hasOrders := root["orders"]
	if hasOrder == hasOrders {
		return errors.New("order response must contain exactly one of order or orders")
	}

	items := []json.RawMessage{orderRaw}
	if hasOrders {
		if bytes.Equal(bytes.TrimSpace(ordersRaw), []byte("null")) {
			return errors.New("orders is null")
		}
		if err := json.Unmarshal(ordersRaw, &items); err != nil {
			return fmt.Errorf("orders must be an array: %w", err)
		}
	}
	if len(items) != 1 {
		return fmt.Errorf("order response returned %d records, want exactly 1", len(items))
	}

	order, err := localnetSemanticDecodeOrder(items[0])
	if err != nil {
		return err
	}
	if order != expected {
		return fmt.Errorf("order = %+v, want %+v", order, expected)
	}
	return nil
}

func localnetSemanticDecodeOrder(data []byte) (localnetSemanticOrder, error) {
	object, err := localnetSemanticJSONObject(data)
	if err != nil {
		return localnetSemanticOrder{}, err
	}
	identity, err := localnetSemanticJSONObjectField(object, "id")
	if err != nil {
		return localnetSemanticOrder{}, err
	}
	owner, err := localnetSemanticRequiredString(identity, "owner")
	if err != nil {
		return localnetSemanticOrder{}, err
	}
	if err := localnetSemanticValidateAddress(owner); err != nil {
		return localnetSemanticOrder{}, err
	}
	dseq, err := localnetSemanticPositiveUint(identity, "dseq", 64)
	if err != nil {
		return localnetSemanticOrder{}, err
	}
	gseq, err := localnetSemanticPositiveUint(identity, "gseq", 32)
	if err != nil {
		return localnetSemanticOrder{}, err
	}
	oseq, err := localnetSemanticPositiveUint(identity, "oseq", 32)
	if err != nil {
		return localnetSemanticOrder{}, err
	}
	state, err := localnetSemanticRequiredString(object, "state")
	if err != nil {
		return localnetSemanticOrder{}, err
	}
	if !localnetSemanticStringIn(state, "open", "active", "closed") {
		return localnetSemanticOrder{}, fmt.Errorf("invalid order state %q", state)
	}

	return localnetSemanticOrder{
		Owner: owner,
		DSeq:  dseq,
		GSeq:  uint32(gseq),
		OSeq:  uint32(oseq),
		State: state,
	}, nil
}

func localnetSemanticAssertGroup(data []byte, expected localnetSemanticGroup) error {
	root, err := localnetSemanticJSONObject(data)
	if err != nil {
		return err
	}
	group, err := localnetSemanticJSONObjectField(root, "group")
	if err != nil {
		return err
	}
	identity, err := localnetSemanticJSONObjectField(group, "id")
	if err != nil {
		return err
	}
	owner, err := localnetSemanticRequiredString(identity, "owner")
	if err != nil {
		return err
	}
	if err := localnetSemanticValidateAddress(owner); err != nil {
		return err
	}
	dseq, err := localnetSemanticPositiveUint(identity, "dseq", 64)
	if err != nil {
		return err
	}
	gseq, err := localnetSemanticPositiveUint(identity, "gseq", 32)
	if err != nil {
		return err
	}
	state, err := localnetSemanticRequiredString(group, "state")
	if err != nil {
		return err
	}
	if !localnetSemanticStringIn(state, "open", "paused", "insufficient_funds", "closed") {
		return fmt.Errorf("invalid group state %q", state)
	}

	actual := localnetSemanticGroup{Owner: owner, DSeq: dseq, GSeq: uint32(gseq), State: state}
	if actual != expected {
		return fmt.Errorf("group = %+v, want %+v", actual, expected)
	}
	return nil
}

func localnetSemanticAssertEscrow(data []byte, expected localnetSemanticEscrow) error {
	if err := localnetSemanticValidateAddress(expected.Owner); err != nil {
		return fmt.Errorf("invalid expected escrow owner: %w", err)
	}
	if expected.DSeq == 0 || expected.Denom == "" {
		return errors.New("expected escrow identity or denomination is empty")
	}
	if !localnetSemanticStringIn(expected.State, "open", "closed", "overdrawn") {
		return fmt.Errorf("invalid expected escrow state %q", expected.State)
	}
	if err := localnetSemanticValidateCanonicalAmount(expected.Amount); err != nil {
		return fmt.Errorf("invalid expected escrow amount: %w", err)
	}

	root, err := localnetSemanticJSONObject(data)
	if err != nil {
		return err
	}
	items, err := localnetSemanticJSONArrayField(root, "accounts")
	if err != nil {
		return err
	}
	if len(items) != 1 {
		return fmt.Errorf("escrow response returned %d accounts, want exactly 1", len(items))
	}

	account, err := localnetSemanticJSONObject(items[0])
	if err != nil {
		return err
	}
	identity, err := localnetSemanticJSONObjectField(account, "id")
	if err != nil {
		return err
	}
	scope, err := localnetSemanticRequiredString(identity, "scope")
	if err != nil {
		return err
	}
	if scope != "deployment" {
		return fmt.Errorf("escrow scope = %q, want deployment", scope)
	}
	xid, err := localnetSemanticRequiredString(identity, "xid")
	if err != nil {
		return err
	}
	wantXID := expected.Owner + "/" + strconv.FormatUint(expected.DSeq, 10)
	if xid != wantXID {
		return fmt.Errorf("escrow xid = %q, want %q", xid, wantXID)
	}

	state, err := localnetSemanticJSONObjectField(account, "state")
	if err != nil {
		return err
	}
	owner, err := localnetSemanticRequiredString(state, "owner")
	if err != nil {
		return err
	}
	if owner != expected.Owner {
		return fmt.Errorf("escrow owner = %q, want %q", owner, expected.Owner)
	}
	stateName, err := localnetSemanticRequiredString(state, "state")
	if err != nil {
		return err
	}
	if stateName != expected.State {
		return fmt.Errorf("escrow state = %q, want %q", stateName, expected.State)
	}

	funds, err := localnetSemanticJSONArrayField(state, "funds")
	if err != nil {
		return err
	}
	if len(funds) != 1 {
		return fmt.Errorf("escrow response returned %d balances, want exactly 1", len(funds))
	}
	coin, err := localnetSemanticJSONObject(funds[0])
	if err != nil {
		return err
	}
	denom, err := localnetSemanticRequiredString(coin, "denom")
	if err != nil {
		return err
	}
	amount, err := localnetSemanticRequiredString(coin, "amount")
	if err != nil {
		return err
	}
	if err := localnetSemanticValidateCanonicalAmount(amount); err != nil {
		return err
	}
	if denom != expected.Denom || amount != expected.Amount {
		return fmt.Errorf("escrow balance = %s%s, want %s%s", amount, denom, expected.Amount, expected.Denom)
	}

	return nil
}

func localnetSemanticAssertMutationBinding(receiptJSON, actionLogJSON []byte, expected localnetSemanticMutation) error {
	wantHash, err := localnetSemanticNormalizeHash(expected.TxHash)
	if err != nil {
		return fmt.Errorf("invalid expected transaction hash: %w", err)
	}
	if expected.Account == "" || expected.Action == "" {
		return errors.New("expected account and action are required")
	}
	if !localnetSemanticStringIn(expected.Status, "success", "failed") {
		return fmt.Errorf("invalid expected transaction status %q", expected.Status)
	}
	if (expected.Code == 0) != (expected.Status == "success") {
		return errors.New("expected code and status are inconsistent")
	}

	receipt, err := localnetSemanticJSONObject(receiptJSON)
	if err != nil {
		return fmt.Errorf("decode native transaction receipt: %w", err)
	}
	receiptHash, err := localnetSemanticRequiredString(receipt, "txhash")
	if err != nil {
		return fmt.Errorf("native transaction receipt: %w", err)
	}
	receiptHash, err = localnetSemanticNormalizeHash(receiptHash)
	if err != nil {
		return fmt.Errorf("native transaction receipt: %w", err)
	}
	if receiptHash != wantHash {
		return fmt.Errorf("native receipt hash = %s, want %s", receiptHash, wantHash)
	}
	height, err := localnetSemanticPositiveUint(receipt, "height", 64)
	if err != nil {
		return fmt.Errorf("native transaction receipt: %w", err)
	}
	if height == 0 {
		return errors.New("native transaction receipt has no committed height")
	}
	code, err := localnetSemanticUint(receipt, "code", 32)
	if err != nil {
		return fmt.Errorf("native transaction receipt: %w", err)
	}
	if uint32(code) != expected.Code {
		return fmt.Errorf("native receipt code = %d, want %d", code, expected.Code)
	}
	receiptStatus := "success"
	if code != 0 {
		receiptStatus = "failed"
	}
	if receiptStatus != expected.Status {
		return fmt.Errorf("native receipt status = %s, want %s", receiptStatus, expected.Status)
	}

	var entries []json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(actionLogJSON))
	if err := decoder.Decode(&entries); err != nil {
		return fmt.Errorf("decode action log array: %w", err)
	}
	if err := localnetSemanticRequireEOF(decoder); err != nil {
		return fmt.Errorf("decode action log array: %w", err)
	}

	matches := 0
	for index, raw := range entries {
		entry, err := localnetSemanticJSONObject(raw)
		if err != nil {
			return fmt.Errorf("action log entry %d: %w", index, err)
		}
		rawHash, exists := entry["tx_hash"]
		if !exists {
			continue
		}
		hash, err := localnetSemanticDecodeString(rawHash)
		if err != nil {
			return fmt.Errorf("action log entry %d tx_hash: %w", index, err)
		}
		hash, err = localnetSemanticNormalizeHash(hash)
		if err != nil {
			return fmt.Errorf("action log entry %d tx_hash: %w", index, err)
		}
		if hash != wantHash {
			continue
		}
		matches++
		entryType, err := localnetSemanticRequiredString(entry, "type")
		if err != nil || entryType != "tx" {
			return fmt.Errorf("bound action entry type = %q, want tx", entryType)
		}
		action, err := localnetSemanticRequiredString(entry, "action")
		if err != nil {
			return fmt.Errorf("bound action entry: %w", err)
		}
		account, err := localnetSemanticRequiredString(entry, "account")
		if err != nil {
			return fmt.Errorf("bound action entry: %w", err)
		}
		status, err := localnetSemanticRequiredString(entry, "status")
		if err != nil {
			return fmt.Errorf("bound action entry: %w", err)
		}
		entryCode := uint64(0)
		if _, exists := entry["code"]; exists {
			entryCode, err = localnetSemanticUint(entry, "code", 32)
			if err != nil {
				return fmt.Errorf("bound action entry: %w", err)
			}
		}
		if action != expected.Action || account != expected.Account || status != expected.Status || uint32(entryCode) != expected.Code {
			return fmt.Errorf(
				"bound action = action %q account %q status %q code %d, want %q %q %q %d",
				action,
				account,
				status,
				entryCode,
				expected.Action,
				expected.Account,
				expected.Status,
				expected.Code,
			)
		}
	}
	if matches != 1 {
		return fmt.Errorf("action log contains %d entries for transaction %s, want exactly 1", matches, wantHash)
	}

	return nil
}

func localnetSemanticJSONObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("value is not a JSON object")
	}
	if err := localnetSemanticRequireEOF(decoder); err != nil {
		return nil, err
	}
	return object, nil
}

func localnetSemanticRequireEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("JSON contains a trailing value")
}

func localnetSemanticJSONObjectField(object map[string]json.RawMessage, field string) (map[string]json.RawMessage, error) {
	raw, exists := object[field]
	if !exists {
		return nil, fmt.Errorf("missing %s", field)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("%s is null", field)
	}
	nested, err := localnetSemanticJSONObject(raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be an object: %w", field, err)
	}
	return nested, nil
}

func localnetSemanticJSONArrayField(object map[string]json.RawMessage, field string) ([]json.RawMessage, error) {
	raw, exists := object[field]
	if !exists {
		return nil, fmt.Errorf("missing %s", field)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("%s is null", field)
	}
	var result []json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("%s must be an array: %w", field, err)
	}
	return result, nil
}

func localnetSemanticRequiredString(object map[string]json.RawMessage, field string) (string, error) {
	raw, exists := object[field]
	if !exists {
		return "", fmt.Errorf("missing %s", field)
	}
	value, err := localnetSemanticDecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	if value == "" || strings.TrimSpace(value) != value {
		return "", fmt.Errorf("%s is empty or contains surrounding whitespace", field)
	}
	return value, nil
}

func localnetSemanticDecodeString(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", errors.New("value must be a string")
	}
	return value, nil
}

func localnetSemanticPositiveUint(object map[string]json.RawMessage, field string, bitSize int) (uint64, error) {
	value, err := localnetSemanticUint(object, field, bitSize)
	if err != nil {
		return 0, err
	}
	if value == 0 {
		return 0, fmt.Errorf("%s must be positive", field)
	}
	return value, nil
}

func localnetSemanticUint(object map[string]json.RawMessage, field string, bitSize int) (uint64, error) {
	raw, exists := object[field]
	if !exists {
		return 0, fmt.Errorf("missing %s", field)
	}
	value := strings.TrimSpace(string(raw))
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return 0, fmt.Errorf("%s must be an unsigned integer", field)
		}
		value = decoded
	}
	parsed, err := strconv.ParseUint(value, 10, bitSize)
	if err != nil || strconv.FormatUint(parsed, 10) != value {
		return 0, fmt.Errorf("%s must be a canonical unsigned integer", field)
	}
	return parsed, nil
}

func localnetSemanticValidateAddress(address string) error {
	hrp, data, err := bech32.DecodeAndConvert(address)
	if err != nil {
		return fmt.Errorf("invalid account address %q", address)
	}
	if hrp != "akash" || len(data) != 20 {
		return fmt.Errorf("account address %q is not a full akash account address", address)
	}
	canonical, err := bech32.ConvertAndEncode(hrp, data)
	if err != nil || canonical != address {
		return fmt.Errorf("account address %q is not canonical", address)
	}
	return nil
}

func localnetSemanticValidateCanonicalAmount(amount string) error {
	value, err := math.LegacyNewDecFromStr(amount)
	if err != nil || value.IsNegative() {
		return fmt.Errorf("invalid non-negative decimal amount %q", amount)
	}
	if value.String() != amount {
		return fmt.Errorf("amount %q is not canonical; want %q", amount, value.String())
	}
	return nil
}

func localnetSemanticNormalizeHash(hash string) (string, error) {
	if len(hash) != 64 {
		return "", fmt.Errorf("transaction hash has %d hex characters, want 64", len(hash))
	}
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("transaction hash is not 32 bytes of hexadecimal")
	}
	return strings.ToUpper(hash), nil
}

func localnetSemanticStringIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func localnetSemanticTestAddress(t *testing.T, seed byte) string {
	t.Helper()
	address, err := bech32.ConvertAndEncode("akash", bytes.Repeat([]byte{seed}, 20))
	if err != nil {
		t.Fatalf("create semantic test address: %v", err)
	}
	return address
}

func TestLocalnetSemanticDeploymentCollections(t *testing.T) {
	t.Parallel()
	owner := localnetSemanticTestAddress(t, 1)
	other := localnetSemanticTestAddress(t, 2)
	entry := func(entryOwner, dseq, state string) string {
		return fmt.Sprintf(`{"deployment":{"id":{"owner":%q,"dseq":%q},"state":%q}}`, entryOwner, dseq, state)
	}
	collection := func(entries ...string) string {
		return `{"deployments":[` + strings.Join(entries, ",") + `],"pagination":{"next_key":""}}`
	}
	valid := collection(entry(owner, "7", "active"))
	want := []localnetSemanticDeployment{{Owner: owner, DSeq: 7, State: "active"}}

	tests := []struct {
		name       string
		aktJSON    string
		nativeJSON string
		wantError  bool
	}{
		{name: "exact semantic match", aktJSON: valid, nativeJSON: valid},
		{name: "missing collection", aktJSON: `{}`, nativeJSON: valid, wantError: true},
		{name: "missing owner", aktJSON: collection(`{"deployment":{"id":{"dseq":"7"},"state":"active"}}`), nativeJSON: valid, wantError: true},
		{name: "malformed owner", aktJSON: collection(entry("akash1bad", "7", "active")), nativeJSON: valid, wantError: true},
		{name: "wrong owner", aktJSON: collection(entry(other, "7", "active")), nativeJSON: valid, wantError: true},
		{name: "missing dseq", aktJSON: collection(fmt.Sprintf(`{"deployment":{"id":{"owner":%q},"state":"active"}}`, owner)), nativeJSON: valid, wantError: true},
		{name: "malformed dseq", aktJSON: collection(entry(owner, "07", "active")), nativeJSON: valid, wantError: true},
		{name: "wrong dseq", aktJSON: collection(entry(owner, "8", "active")), nativeJSON: valid, wantError: true},
		{name: "wrong state", aktJSON: collection(entry(owner, "7", "closed")), nativeJSON: valid, wantError: true},
		{name: "duplicate identity", aktJSON: collection(entry(owner, "7", "active"), entry(owner, "7", "active")), nativeJSON: valid, wantError: true},
		{
			name:       "expected text in unrelated field",
			aktJSON:    fmt.Sprintf(`{"deployments":[{"deployment":{"id":{"owner":%q,"dseq":"8"},"state":"closed"},"note":%q}]}`, other, owner+`/7 active`),
			nativeJSON: valid,
			wantError:  true,
		},
		{name: "native observer mismatch", aktJSON: valid, nativeJSON: collection(entry(owner, "7", "closed")), wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := localnetSemanticAssertDeploymentCollections([]byte(test.aktJSON), []byte(test.nativeJSON), want)
			if (err != nil) != test.wantError {
				t.Fatalf("localnetSemanticAssertDeploymentCollections() error = %v, wantError %t", err, test.wantError)
			}
		})
	}
}

func TestLocalnetSemanticOrder(t *testing.T) {
	t.Parallel()
	owner := localnetSemanticTestAddress(t, 3)
	other := localnetSemanticTestAddress(t, 4)
	want := localnetSemanticOrder{Owner: owner, DSeq: 7, GSeq: 1, OSeq: 1, State: "open"}
	order := func(entryOwner, dseq, gseq, oseq, state string) string {
		return fmt.Sprintf(`{"id":{"owner":%q,"dseq":%q,"gseq":%s,"oseq":%s},"state":%q}`, entryOwner, dseq, gseq, oseq, state)
	}
	validOrder := order(owner, "7", "1", "1", "open")

	tests := []struct {
		name      string
		body      string
		wantError bool
	}{
		{name: "detail", body: `{"order":` + validOrder + `}`},
		{name: "single-item collection", body: `{"orders":[` + validOrder + `]}`},
		{name: "missing identity", body: `{"order":{"state":"open"}}`, wantError: true},
		{name: "wrong owner", body: `{"order":` + order(other, "7", "1", "1", "open") + `,"note":` + strconv.Quote(owner) + `}`, wantError: true},
		{name: "malformed dseq", body: `{"order":` + order(owner, "seven", "1", "1", "open") + `}`, wantError: true},
		{name: "wrong gseq", body: `{"order":` + order(owner, "7", "2", "1", "open") + `}`, wantError: true},
		{name: "wrong oseq", body: `{"order":` + order(owner, "7", "1", "2", "open") + `}`, wantError: true},
		{name: "wrong state", body: `{"order":` + order(owner, "7", "1", "1", "closed") + `}`, wantError: true},
		{name: "duplicate collection identity", body: `{"orders":[` + validOrder + `,` + validOrder + `]}`, wantError: true},
		{name: "identity only in note", body: fmt.Sprintf(`{"order":%s,"note":%q}`, order(other, "8", "2", "2", "closed"), owner+`/7/1/1 open`), wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := localnetSemanticAssertOrder([]byte(test.body), want)
			if (err != nil) != test.wantError {
				t.Fatalf("localnetSemanticAssertOrder() error = %v, wantError %t", err, test.wantError)
			}
		})
	}
}

func TestLocalnetSemanticGroup(t *testing.T) {
	t.Parallel()
	owner := localnetSemanticTestAddress(t, 5)
	other := localnetSemanticTestAddress(t, 6)
	want := localnetSemanticGroup{Owner: owner, DSeq: 7, GSeq: 1, State: "open"}
	group := func(entryOwner, dseq, gseq, state string) string {
		return fmt.Sprintf(`{"group":{"id":{"owner":%q,"dseq":%q,"gseq":%s},"state":%q}}`, entryOwner, dseq, gseq, state)
	}

	tests := []struct {
		name      string
		body      string
		wantError bool
	}{
		{name: "exact group", body: group(owner, "7", "1", "open")},
		{name: "missing group", body: `{}`, wantError: true},
		{name: "missing identity", body: `{"group":{"state":"open"}}`, wantError: true},
		{name: "malformed owner", body: group("akash1bad", "7", "1", "open"), wantError: true},
		{name: "wrong owner", body: group(other, "7", "1", "open"), wantError: true},
		{name: "malformed dseq", body: group(owner, "7x", "1", "open"), wantError: true},
		{name: "wrong dseq", body: group(owner, "8", "1", "open"), wantError: true},
		{name: "wrong gseq", body: group(owner, "7", "2", "open"), wantError: true},
		{name: "wrong state", body: group(owner, "7", "1", "closed"), wantError: true},
		{name: "identity only in note", body: fmt.Sprintf(`{"group":{"id":{"owner":%q,"dseq":"8","gseq":2},"state":"closed","note":%q}}`, other, owner+`/7/1 open`), wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := localnetSemanticAssertGroup([]byte(test.body), want)
			if (err != nil) != test.wantError {
				t.Fatalf("localnetSemanticAssertGroup() error = %v, wantError %t", err, test.wantError)
			}
		})
	}
}

func TestLocalnetSemanticEscrow(t *testing.T) {
	t.Parallel()
	owner := localnetSemanticTestAddress(t, 7)
	other := localnetSemanticTestAddress(t, 8)
	amount := math.LegacyMustNewDecFromStr("5000000").String()
	want := localnetSemanticEscrow{Owner: owner, DSeq: 7, State: "open", Denom: "uact", Amount: amount}
	account := func(entryOwner, xid, state, denom, entryAmount string) string {
		return fmt.Sprintf(
			`{"id":{"scope":"deployment","xid":%q},"state":{"owner":%q,"state":%q,"funds":[{"denom":%q,"amount":%q}]}}`,
			xid,
			entryOwner,
			state,
			denom,
			entryAmount,
		)
	}
	validAccount := account(owner, owner+"/7", "open", "uact", amount)

	tests := []struct {
		name      string
		body      string
		wantError bool
	}{
		{name: "exact account and balance", body: `{"accounts":[` + validAccount + `]}`},
		{name: "missing accounts", body: `{}`, wantError: true},
		{name: "duplicate account", body: `{"accounts":[` + validAccount + `,` + validAccount + `]}`, wantError: true},
		{name: "wrong scope", body: strings.Replace(validAccount, `"scope":"deployment"`, `"scope":"bid"`, 1), wantError: true},
		{name: "wrong xid owner", body: `{"accounts":[` + account(owner, other+"/7", "open", "uact", amount) + `]}`, wantError: true},
		{name: "malformed xid dseq", body: `{"accounts":[` + account(owner, owner+"/seven", "open", "uact", amount) + `]}`, wantError: true},
		{name: "wrong state owner", body: `{"accounts":[` + account(other, owner+"/7", "open", "uact", amount) + `]}`, wantError: true},
		{name: "wrong state", body: `{"accounts":[` + account(owner, owner+"/7", "closed", "uact", amount) + `]}`, wantError: true},
		{name: "wrong denom", body: `{"accounts":[` + account(owner, owner+"/7", "open", "uakt", amount) + `]}`, wantError: true},
		{name: "wrong amount", body: `{"accounts":[` + account(owner, owner+"/7", "open", "uact", math.LegacyMustNewDecFromStr("4999999").String()) + `]}`, wantError: true},
		{name: "noncanonical amount", body: `{"accounts":[` + account(owner, owner+"/7", "open", "uact", "5000000") + `]}`, wantError: true},
		{name: "identity and amount only in note", body: fmt.Sprintf(`{"accounts":[%s],"note":%q}`, account(other, other+"/8", "closed", "uakt", math.LegacyMustNewDecFromStr("1").String()), owner+`/7 open 5000000uact`), wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := localnetSemanticAssertEscrow([]byte(test.body), want)
			if (err != nil) != test.wantError {
				t.Fatalf("localnetSemanticAssertEscrow() error = %v, wantError %t", err, test.wantError)
			}
		})
	}
}

func TestLocalnetSemanticMutationBinding(t *testing.T) {
	t.Parallel()
	hash := strings.Repeat("A1", 32)
	otherHash := strings.Repeat("B2", 32)
	want := localnetSemanticMutation{
		TxHash:  hash,
		Code:    0,
		Status:  "success",
		Account: "validator",
		Action:  "deployment.MsgCreateDeployment",
	}
	receipt := fmt.Sprintf(`{"txhash":%q,"height":"17","code":0}`, hash)
	entry := func(entryHash string, code uint32, status, account, action string) string {
		return fmt.Sprintf(`{"type":"tx","tx_hash":%q,"code":%d,"status":%q,"account":%q,"action":%q}`, entryHash, code, status, account, action)
	}
	validEntry := entry(strings.ToLower(hash), 0, "success", "validator", want.Action)
	unrelatedEntry := entry(otherHash, 0, "success", "validator", "bank.MsgSend")

	tests := []struct {
		name      string
		receipt   string
		log       string
		wantError bool
	}{
		{name: "exact binding with unrelated entry", receipt: receipt, log: `[` + unrelatedEntry + `,` + validEntry + `]`},
		{name: "missing receipt hash", receipt: `{"height":"17","code":0}`, log: `[` + validEntry + `]`, wantError: true},
		{name: "wrong receipt hash", receipt: fmt.Sprintf(`{"txhash":%q,"height":"17","code":0,"note":%q}`, otherHash, hash), log: `[` + validEntry + `]`, wantError: true},
		{name: "malformed receipt hash", receipt: fmt.Sprintf(`{"txhash":%q,"height":"17","code":0}`, hash+"00"), log: `[` + validEntry + `]`, wantError: true},
		{name: "uncommitted receipt", receipt: fmt.Sprintf(`{"txhash":%q,"height":"0","code":0}`, hash), log: `[` + validEntry + `]`, wantError: true},
		{name: "wrong receipt code", receipt: fmt.Sprintf(`{"txhash":%q,"height":"17","code":7}`, hash), log: `[` + validEntry + `]`, wantError: true},
		{name: "no matching action", receipt: receipt, log: `[` + unrelatedEntry + `]`, wantError: true},
		{name: "duplicate matching action", receipt: receipt, log: `[` + validEntry + `,` + validEntry + `]`, wantError: true},
		{name: "wrong action", receipt: receipt, log: `[` + entry(hash, 0, "success", "validator", "bank.MsgSend") + `]`, wantError: true},
		{name: "wrong account", receipt: receipt, log: `[` + entry(hash, 0, "success", "other", want.Action) + `]`, wantError: true},
		{name: "wrong status", receipt: receipt, log: `[` + entry(hash, 0, "pending", "validator", want.Action) + `]`, wantError: true},
		{name: "wrong action code", receipt: receipt, log: `[` + entry(hash, 7, "success", "validator", want.Action) + `]`, wantError: true},
		{name: "hash and action only in error", receipt: receipt, log: fmt.Sprintf(`[{"type":"tx","tx_hash":%q,"status":"success","account":"validator","action":"bank.MsgSend","error":%q}]`, otherHash, hash+" "+want.Action), wantError: true},
		{name: "substring hash is not identity", receipt: receipt, log: fmt.Sprintf(`[{"type":"tx","tx_hash":%q,"status":"success","account":"validator","action":%q}]`, hash+"00", want.Action), wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := localnetSemanticAssertMutationBinding([]byte(test.receipt), []byte(test.log), want)
			if (err != nil) != test.wantError {
				t.Fatalf("localnetSemanticAssertMutationBinding() error = %v, wantError %t", err, test.wantError)
			}
		})
	}
}

func TestLocalnetRawActionLogOracleCollapsesTransactionRevisions(t *testing.T) {
	t.Parallel()
	firstTimestamp := "2026-08-11T10:00:00Z"
	raw := strings.Join([]string{
		`{"ts":"2026-08-11T09:00:00Z","type":"context","action":"create"}`,
		`{"ts":"` + firstTimestamp + `","type":"tx","action":"bank.MsgSend","tx_hash":"AA","status":"pending","account":"validator"}`,
		`{"ts":"2026-08-11T10:01:00Z","type":"tx","action":"deployment.MsgCreateDeployment","tx_hash":"BB","status":"success","account":"validator","height":9}`,
		`{"ts":"2026-08-11T10:02:00Z","type":"tx","action":"bank.MsgSend","tx_hash":"AA","status":"success","account":"validator","height":10,"gas_used":50}`,
		"",
	}, "\n")

	entries, err := decodeLocalnetRawTxActions([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].TxHash != "BB" || entries[1].TxHash != "AA" {
		t.Fatalf("collapsed raw actions = %+v, want reverse submission order BB, AA", entries)
	}
	if entries[1].Status != "success" || entries[1].Height != 10 || entries[1].GasUsed != 50 || entries[1].Timestamp.Format(time.RFC3339) != firstTimestamp {
		t.Fatalf("collapsed AA action = %+v, want terminal fields with original timestamp", entries[1])
	}
	if count, err := countLocalnetRawActionEntries([]byte(raw)); err != nil || count != 4 {
		t.Fatalf("raw action count = %d, %v, want 4", count, err)
	}
}

func TestLocalnetRawActionLogOracleRejectsMalformedJSONL(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		"{not-json}\n",
		"[]\n",
	} {
		if _, err := countLocalnetRawActionEntries([]byte(data)); err == nil {
			t.Fatalf("countLocalnetRawActionEntries(%q) unexpectedly succeeded", data)
		}
	}
	if _, err := decodeLocalnetRawTxActions([]byte("{not-json}\n")); err == nil {
		t.Fatal("decodeLocalnetRawTxActions accepted malformed JSONL")
	}
}

func TestLocalnetSemanticSuccessfulJSONContract(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		stdout    string
		stderr    string
		exitCode  int
		wantError bool
	}{
		{name: "object on stdout only", stdout: "  {\"state\":\"active\"}\n"},
		{name: "nonzero exit", stdout: `{"state":"active"}`, exitCode: 1, wantError: true},
		{name: "stderr diagnostic", stdout: `{"state":"active"}`, stderr: "warning: stale state\n", wantError: true},
		{name: "JSON hidden on stderr", stdout: "not json", stderr: `{"state":"active"}`, wantError: true},
		{name: "trailing JSON document", stdout: "{\"state\":\"active\"}\n{\"state\":\"closed\"}", wantError: true},
		{name: "JSON scalar", stdout: `"active"`, wantError: true},
		{name: "empty object", stdout: `{}`, wantError: true},
		{name: "empty stdout", wantError: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := localnetSemanticValidateSuccessfulJSON(test.stdout, test.stderr, test.exitCode)
			if (err != nil) != test.wantError {
				t.Fatalf("localnetSemanticValidateSuccessfulJSON() error = %v, wantError %t", err, test.wantError)
			}
		})
	}
}
