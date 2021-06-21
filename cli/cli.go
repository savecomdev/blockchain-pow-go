package cli

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"

	"github.com/savecomdev/blockchain-pow-go/blockchain"
	"github.com/savecomdev/blockchain-pow-go/network"
	"github.com/savecomdev/blockchain-pow-go/wallet"
)

type CommandLine struct{}

func (cli *CommandLine) printUsage() {
	fmt.Println("Usage commandes :")
	fmt.Println("--> To get the balance for the account: \ngetbalance -address ADDRESS")
	fmt.Println("--> To create a chain: \ncreateblockchain -address ADDRESS")
	fmt.Println("--> To prints the blocks in the chain: \nprintchain")
	fmt.Println("--> To send amount from account to another into the chain. The -mine flag indicate that node mining kind:	\nsend -from FROM -to TO -amount AMOUNT -mine")
	fmt.Println("--> To creates a new wallet: \ncreatewallet")
	fmt.Println("--> To list the addresses in our waller file: \nlistaddresses")
	fmt.Println("--> To rebuild the UTXO set: \nreindexutxo")
	fmt.Println("--> To start a node with ID specified in NODE_ID env. var. -miner enables mining: \nstartnode -miner ADDRESS")
}

func (cli *CommandLine) validateArgs() {
	if len(os.Args) < 2 {
		cli.printUsage()
		runtime.Goexit()
	}
}

func (cli *CommandLine) printChain(nodeID string) {
	chain := blockchain.CountinueBlockChain(nodeID)
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

		for _, tx := range block.Transactions {
			fmt.Println(tx)
		}

		fmt.Println()

		// leave the loop at the last block
		if len(block.PrevHash) == 0 {
			break
		}
	}
}

func (cli *CommandLine) createBlockChain(address, nodeID string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("Address isn't valid !!!")
	}

	chain := blockchain.InitBlockChain(address, nodeID)
	chain.Database.Close()

	UTXOSet := blockchain.UTXOSet{chain}
	UTXOSet.Reindex()

	fmt.Println("Finished !!!")
}

func (cli *CommandLine) getBalance(address, nodeID string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("Address isn't valid !!!")
	}

	// open the current chain
	chain := blockchain.CountinueBlockChain(nodeID)
	UTXOSet := blockchain.UTXOSet{chain}
	defer chain.Database.Close()

	balance := 0
	pubKeyHash := wallet.Base58Decode([]byte(address))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]
	UTXOs := UTXOSet.FindUnspentTransactions(pubKeyHash)

	for _, out := range UTXOs {
		balance += out.Value
	}

	fmt.Printf("Balance of %s: %d\n", address, balance)
}

func (cli *CommandLine) send(from, to string, amount int, nodeID string, mineNow bool) {
	if !wallet.ValidateAddress(from) {
		log.Panic("Address isn't valid !!!")
	}
	if !wallet.ValidateAddress(to) {
		log.Panic("Address isn't valid !!!")
	}

	// open the current chain
	chain := blockchain.CountinueBlockChain(nodeID)
	UTXOSet := blockchain.UTXOSet{chain}
	defer chain.Database.Close()

	wallets, err := wallet.CreateWallets(nodeID)
	blockchain.ErrorHandler(err)
	wallet := wallets.GetWallet(from)

	tx := blockchain.NewTransaction(&wallet, to, amount, &UTXOSet)
	if mineNow {
		cbTx := blockchain.CoinBaseTx(from, "")
		txs := []*blockchain.Transaction{cbTx, tx}
		block := chain.MineBlock(txs)
		UTXOSet.Update(block)

	} else {
		network.SendTransaction(network.KnownNodes[0], tx)
		fmt.Println("Send transaction")
	}

	fmt.Println("Sending with success !!!")
}

func (cli *CommandLine) listAddresses(nodeID string) {
	wallets, _ := wallet.CreateWallets(nodeID)
	addresses := wallets.GetAllAddresses()

	for _, address := range addresses {
		fmt.Println(address)
	}
}

func (cli *CommandLine) createWallet(nodeID string) {
	wallets, _ := wallet.CreateWallets(nodeID)
	address := wallets.AddWallet()

	wallets.SaveIntoFile(nodeID)
	fmt.Printf("Create new wallet with address: %s\n", address)
}

func (cli *CommandLine) reindexUTXO(nodeID string) {
	chain := blockchain.CountinueBlockChain(nodeID)
	defer chain.Database.Close()

	UTXOSet := blockchain.UTXOSet{chain}
	UTXOSet.Reindex()

	count := UTXOSet.CountTransactions()
	fmt.Printf("Done! There are %d transactions in the UTXOset.\n", count)
}

func (cli *CommandLine) StartNode(nodeID, minerAddress string) {
	fmt.Printf("Starting Node %s\n", nodeID)

	if len(minerAddress) > 0 {
		if wallet.ValidateAddress(minerAddress) {
			fmt.Println("Mining is on. Address to receive rewards: ", minerAddress)
		} else {
			log.Panic("Wrong miner address!")
		}
	}
	network.StartServer(nodeID, minerAddress)
}

// main function of the cli
func (cli *CommandLine) Run() {
	cli.validateArgs()

	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		fmt.Printf("NODE_ID env is not set !")
		runtime.Goexit()
	}

	// cmd
	getBalanceCmd := flag.NewFlagSet("getbalance", flag.ExitOnError)
	createBlockChainCmd := flag.NewFlagSet("createblockchain", flag.ExitOnError)
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("printchain", flag.ExitOnError)
	createwalletCmd := flag.NewFlagSet("createwallet", flag.ExitOnError)
	listaddressesCmd := flag.NewFlagSet("listaddresses", flag.ExitOnError)
	reindexutxoCmd := flag.NewFlagSet("reindexutxo", flag.ExitOnError)
	startNodeCmd := flag.NewFlagSet("startnode", flag.ExitOnError)

	// data
	getBalanceAddress := getBalanceCmd.String("address", "", "The address of the wallet")
	createBlockChainAddress := createBlockChainCmd.String("address", "", "The address of the database")
	sendFrom := sendCmd.String("from", "", "The source wallet address")
	sendTo := sendCmd.String("to", "", "The destination wallet address")
	sendAmount := sendCmd.Int("amount", 0, "The amount to send, must be upper than 0 value")
	sendMine := sendCmd.Bool("mine", false, "Mine immediately on the same node")
	startNodeMiner := startNodeCmd.String("miner", "", "Enable mining mode an send reward to the node")

	// get the arguments throw the command
	switch os.Args[1] {
	case "printchain":
		err := printChainCmd.Parse(os.Args[2:])
		blockchain.ErrorHandler(err)
	case "startnode":
		err := startNodeCmd.Parse(os.Args[2:])
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
	case "listaddresses":
		err := listaddressesCmd.Parse(os.Args[2:])
		blockchain.ErrorHandler(err)
	case "createwallet":
		err := createwalletCmd.Parse(os.Args[2:])
		blockchain.ErrorHandler(err)
	case "reindexutxo":
		err := reindexutxoCmd.Parse(os.Args[2:])
		blockchain.ErrorHandler(err)
	default:
		cli.printUsage()
		runtime.Goexit()
	}

	if printChainCmd.Parsed() {
		cli.printChain(nodeID)
	}

	if getBalanceCmd.Parsed() {
		if *getBalanceAddress == "" {
			getBalanceCmd.Usage()
			runtime.Goexit()
		}
		cli.getBalance(*getBalanceAddress, nodeID)
	}

	if createBlockChainCmd.Parsed() {
		if *createBlockChainAddress == "" {
			createBlockChainCmd.Usage()
			runtime.Goexit()
		}
		cli.createBlockChain(*createBlockChainAddress, nodeID)
	}

	if sendCmd.Parsed() {
		if *sendFrom == "" || *sendTo == "" || *sendAmount <= 0 {
			sendCmd.Usage()
			runtime.Goexit()
		}
		cli.send(*sendFrom, *sendTo, *sendAmount, nodeID, *sendMine)
	}

	if createwalletCmd.Parsed() {
		cli.createWallet(nodeID)
	}

	if listaddressesCmd.Parsed() {
		cli.listAddresses(nodeID)
	}

	if reindexutxoCmd.Parsed() {
		cli.reindexUTXO(nodeID)
	}

	if startNodeCmd.Parsed() {
		nodeID := os.Getenv("NODE_ID")
		if nodeID == "" {
			startNodeCmd.Usage()
			runtime.Goexit()
		}

		cli.StartNode(nodeID, *startNodeMiner)
	}
}
