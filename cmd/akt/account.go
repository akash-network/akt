package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/go-bip39"
	"github.com/spf13/cobra"
)

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage accounts",
}

var (
	accountName   string
	accountGlobal bool
	accountType   string
)

func init() {
	accountCmd.AddCommand(createAccountCmd)
	accountCmd.AddCommand(listAccountsCmd)

	createAccountCmd.Flags().BoolVar(&accountGlobal, "global", true, "Create a global account")
	createAccountCmd.Flags().StringVar(&accountType, "type", keyring.BackendTest, "Use the given keyring backend")

	listAccountsCmd.Flags().BoolVar(&accountGlobal, "global", true, "List global accounts")
	listAccountsCmd.Flags().StringVar(&accountType, "type", keyring.BackendTest, "Use the given keyring backend")
}

var createAccountCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new account",
	Args:  cobra.ExactArgs(1),
	RunE:  runCreateAccountCmd,
}

func runCreateAccountCmd(cmd *cobra.Command, args []string) error {
	accountName = args[0]

	keyringDir := getKeyringDir(accountGlobal)
	interfaceRegistry := types.NewInterfaceRegistry()
	std.RegisterInterfaces(interfaceRegistry)
	marshaler := codec.NewProtoCodec(interfaceRegistry)

	kr, err := keyring.New("akash", accountType, keyringDir, os.Stdin, marshaler)
	if err != nil {
		return fmt.Errorf("failed to initialize keyring: %w", err)
	}

	// Generate a mnemonic
	entropySeed, err := bip39.NewEntropy(256)
	if err != nil {
		return fmt.Errorf("failed to generate entropy seed: %w", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropySeed)
	if err != nil {
		return fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	// Correctly specify the signature algorithm
	algo := hd.Secp256k1

	account, err := kr.NewAccount(accountName, mnemonic, "", sdk.FullFundraiserPath, algo)
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	address, err := account.GetAddress()
	if err != nil {
		return fmt.Errorf("failed to get address for account: %w", err)
	}

	fmt.Printf("Account %s created successfully\n", accountName)
	fmt.Println("\nPlease capture your Mnemonic for future use/account recovery:\n")
	fmt.Println(mnemonic)
	fmt.Println("\nYour account name and address are:\n")
	fmt.Printf("Name: %s\nAddress: %s\n", accountName, address.String())

	return nil
}

var listAccountsCmd = &cobra.Command{
	Use:   "list",
	Short: "List current accounts",
	RunE:  runListAccountsCmd,
}

func runListAccountsCmd(cmd *cobra.Command, args []string) error {
	keyringDir := getKeyringDir(accountGlobal)
	interfaceRegistry := types.NewInterfaceRegistry()
	std.RegisterInterfaces(interfaceRegistry)
	marshaler := codec.NewProtoCodec(interfaceRegistry)

	kr, err := keyring.New("akash", accountType, keyringDir, os.Stdin, marshaler)
	if err != nil {
		return fmt.Errorf("failed to initialize keyring: %w", err)
	}

	infos, err := kr.List()
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	for _, info := range infos {
		address, err := info.GetAddress()
		if err != nil {
			return fmt.Errorf("failed to get address for account %s: %w", info.Name, err)
		}
		fmt.Printf("- name: %s\n  type: %s\n  address: %s\n", info.Name, accountType, address.String())
	}

	return nil
}

func getKeyringDir(global bool) string {
	if global {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("Error getting home directory: %v\n", err)
			os.Exit(1)
		}
		return filepath.Join(homeDir, ".akash")
	}
	return "./.akash"
}
