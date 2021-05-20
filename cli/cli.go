package cli

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"

	"github.com/savecomdev/blockchain-pow-go/blockchain"
)

type CommandLine struct{}

func (cli *CommandLine) printUsage() {
	fmt.Println("Usage commandes :")
	fmt.Println("--> To get the balance for the account: \ngetbalance -address ADDRESS")
	fmt.Println("--> To create a chain: \ncreateblockchain -address ADDRESS")
	fmt.Println("--> To prints the blocks in the chain: \nprintchain")
	fmt.Println("--> To send amount from account to another into the chain:	\nsend -from FROM -to TO -amount AMOUNT")
}

func (cli *CommandLine) validateArgs() {
	if len(os.Args) < 2 {
		cli.printUsage()
		runtime.Goexit()
	}
}

func (cli *CommandLine) printChain() {
	chain := blockchain.CountinueBlockChain("")
	defer chain.Database.Close()

	// get the iterator into the chain
	iter := chain.Iterator()

	// iterate loop
	for {
		block := iter.Next()

		fmt.Printf("Previous Hash: %x\n", block.PrevHash)
		fmt.Printf("Hash: %x\n", block.Hash)

		pow := blockchain.NewProof(block)
		fmt.Printf("PoW: %s\n", strconv.FormatBool(pow.Validate()))

		fmt.Println()

		// leave the loop at the last block
		if len(block.PrevHash) == 0 {
			break
		}
	}
}

func (cli *CommandLine) createBlockChain(address string) {
	chain := blockchain.InitBlockChain(address)
	chain.Database.Close()
	fmt.Println("Finished !!!")
}

func (cli *CommandLine) getBalance(address string) {
	// open the current chain
	chain := blockchain.CountinueBlockChain(address)
	defer chain.Database.Close()

	balance := 0
	UTXOs := chain.FindUTXO(address)

	for _, out := range UTXOs {
		balance += out.Value
	}

	fmt.Printf("Balance of %s: %d\n", address, balance)
}

func (cli *CommandLine) send(from, to string, amount int) {
	// open the current chain
	chain := blockchain.CountinueBlockChain(from)
	defer chain.Database.Close()

	tx := blockchain.NewTransaction(from, to, amount, chain)
	chain.AddBlock([]*blockchain.Transaction{tx})
	fmt.Println("Sending with success !!!")
}

// main function of the cli
func (cli *CommandLine) Run() {
	cli.validateArgs()

	// cmd
	getBalanceCmd := flag.NewFlagSet("getbalance", flag.ExitOnError)
	createBlockChainCmd := flag.NewFlagSet("createblockchain", flag.ExitOnError)
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("print", flag.ExitOnError)

	// data
	getBalanceAddress := getBalanceCmd.String("address", "", "The address of the wallet")
	createBlockChainAddress := createBlockChainCmd.String("address", "", "The address of the database")
	sendFrom := sendCmd.String("from", "", "The source wallet address")
	sendTo := sendCmd.String("to", "", "The destination wallet address")
	sendAmount := sendCmd.Int("amount", 0, "The amount to send, must be upper than 0 value")

	// get the arguments throw the command
	switch os.Args[1] {
	case "printchain":
		err := printChainCmd.Parse(os.Args[2:])
		blockchain.ErrorHandler(err)
	case "getbalance":
		err := getBalanceCmd.Parse(os.Args[2:])
		blockchain.ErrorHandler(err)
	case "createblockchain":
		err := createBlockChainCmd.Parse(os.Args[2:])
		blockchain.ErrorHandler(err)
	case "send":
		err := sendCmd.Parse(os.Args[2:])
		blockchain.ErrorHandler(err)
	default:
		cli.printUsage()
		runtime.Goexit()
	}

	if printChainCmd.Parsed() {
		cli.printChain()
	}

	if getBalanceCmd.Parsed() {
		if *getBalanceAddress == "" {
			getBalanceCmd.Usage()
			runtime.Goexit()
		}
		cli.getBalance(*getBalanceAddress)
	}

	if createBlockChainCmd.Parsed() {
		if *createBlockChainAddress == "" {
			createBlockChainCmd.Usage()
			runtime.Goexit()
		}
		cli.createBlockChain(*createBlockChainAddress)
	}

	if sendCmd.Parsed() {
		if *sendFrom == "" || *sendTo == "" || *sendAmount <= 0 {
			sendCmd.Usage()
			runtime.Goexit()
		}
		cli.send(*sendFrom, *sendTo, *sendAmount)
	}
}
