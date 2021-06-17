package network

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime"
	"syscall"

	"github.com/savecomdev/blockchain-pow-go/blockchain"
	"github.com/vrecan/death/v3"
)

const (
	protocol      = "tcp"
	version       = 1
	commandLength = 12
)

var (
	nodeAddress     string
	minerAddress    string
	KnownNodes      = []string{"localhost:3000"}
	blocksInTransit = [][]byte{}
	memoryPool      = make(map[string]blockchain.Transaction)
)

type Addr struct {
	AddrList []string
}

type Block struct {
	AddrFrom string
	Block    []byte
}

type GetBlocks struct {
	AddrFrom string
}

type GetData struct {
	AddrFrom string
	Type     string
	ID       []byte
}

type Inventory struct {
	AddrFrom string
	Type     string
	Items    [][]byte
}

type Tx struct {
	AddrFrom    string
	Transaction []byte
}

type Version struct {
	Version    int
	BestHeight int
	AddrFrom   string
}

func ErrorHandler(err error) {
	if err != nil {
		log.Panic(err)
	}
}

// Convert a command into slice of byte
func CmdToBytes(cmd string) []byte {
	var bytes [commandLength]byte

	for i, c := range cmd {
		bytes[i] = byte(c)
	}

	return bytes[:]
}

// Convert a slice of byte into command
func BytesToCmd(bytes []byte) string {
	var cmd []byte
	for _, b := range bytes {
		if b != 0x0 {
			cmd = append(cmd, b)
		}
	}
	return fmt.Sprintf("%s", cmd)
}

// Serialize the DB
func GobEncode(data interface{}) []byte {
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	err := enc.Encode(data)
	ErrorHandler(err)
	return buff.Bytes()
}

// Close properly the DB
func CloseDB(chain *blockchain.BlockChain) {
	// open the DB
	db := death.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	db.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit()
		chain.Database.Close()
	})
}

// Check if the node address is into the current nodes list
func NodeIsKnown(address string) bool {
	for _, node := range KnownNodes {
		if node == address {
			return true
		}
	}
	return false
}

// Loop to get the blocks from the peer into the pipe network
func RequestBlocks() {
	for _, node := range KnownNodes {
		SendGetBlock(node)
	}
}

func ExtractCmd(request []byte) []byte {
	return request[:commandLength]
}

// Apply the mining process on the chain
func MineTransaction(chain *blockchain.BlockChain) {
	var txs []*blockchain.Transaction

	for id := range memoryPool {
		fmt.Printf("Tx: %s\n", memoryPool[id].ID)
		tx := memoryPool[id]
		if chain.VerifyTransaction(&tx) {
			txs = append(txs, &tx)
		}
	}

	if len(txs) == 0 {
		fmt.Printf("All transaction are invalid")
		return
	}

	// add the initial transaction into the chain
	cbTx := blockchain.CoinBaseTx(minerAddress, "")
	txs = append(txs, cbTx)

	// add new block with the transaction at the end of the chain
	newBlock := chain.MineBlock(txs)
	UTXOSet := blockchain.UTXOSet{chain}
	UTXOSet.Reindex()

	fmt.Printf("New Block mined")

	// clear the memory pool
	for _, tx := range txs {
		txID := hex.EncodeToString(tx.ID)
		delete(memoryPool, txID)
	}

	// push the block to all peer into the network pipe
	for _, node := range KnownNodes {
		if node != nodeAddress {
			SendInventory(node, "block", [][]byte{newBlock.Hash})
		}
	}

	// recursive call of the function
	if len(memoryPool) > 0 {
		MineTransaction(chain)
	}
}

// Push an address into the pipe network
func SendAddr(address string) {
	nodes := Addr{KnownNodes}
	nodes.AddrList = append(nodes.AddrList, nodeAddress)
	payload := GobEncode(nodes)
	request := append(CmdToBytes("addr"), payload...)

	SendData(address, request)
}

// Push a block link into an address into the pipe network
func SendBlock(address string, block *blockchain.Block) {
	data := Block{nodeAddress, block.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("block"), payload...)

	SendData(address, request)
}

// Push an inventory link into an address into the pipe network
func SendInventory(address, kind string, items [][]byte) {
	inventory := Inventory{nodeAddress, kind, items}
	payload := GobEncode(inventory)
	request := append(CmdToBytes("inv"), payload...)

	SendData(address, request)
}

// Push a transaction link into an address into the pipe network
func SendTransaction(adddress string, tnx *blockchain.Transaction) {
	data := Tx{nodeAddress, tnx.Serialize()}
	payload := GobEncode(data)
	request := append(CmdToBytes("tx"), payload...)

	SendData(adddress, request)
}

// Push the chain version number link into an address into the pipe network
func SendVersion(address string, chain *blockchain.BlockChain) {
	bestHeight := chain.GetBestHeight()
	payload := GobEncode(Version{version, bestHeight, nodeAddress})

	request := append(CmdToBytes("version"), payload...)

	SendData(address, request)
}

// Claim the blocks link into an address into the pipe network
func SendGetBlock(address string) {
	payload := GobEncode(GetBlocks{nodeAddress})
	request := append(CmdToBytes("getblocks"), payload...)

	SendData(address, request)
}

// Claim the kind of data link into an address into the pipe network
func SendGetData(address, kind string, id []byte) {
	payload := GobEncode(GetData{nodeAddress, kind, id})
	request := append(CmdToBytes("getdata"), payload...)

	SendData(address, request)
}

// Push data into the pipe network
func SendData(addr string, data []byte) {
	conn, err := net.Dial(protocol, addr)

	if err != nil {
		fmt.Printf("%s si not available\n", addr)

		var updateNodes []string

		for _, node := range KnownNodes {
			if node != addr {
				updateNodes = append(updateNodes, node)
			}
		}

		KnownNodes = updateNodes
		return
	}

	defer conn.Close()

	written, err := io.Copy(conn, bytes.NewReader(data))
	ErrorHandler(err)
	fmt.Printf("Done send data size %x", written)
}

// Maintain the TCP connection and read the content
func HandleConnection(conn net.Conn, chain *blockchain.BlockChain) {
	request, err := ioutil.ReadAll(conn)
	defer conn.Close()

	ErrorHandler(err)

	command := BytesToCmd(request[:commandLength])
	fmt.Printf("Received %s command\n", command)

	switch command {
	case "addr":
		HandleAddress(request)
	case "block":
		HandleBlock(request, chain)
	case "inv":
		HandleInventory(request, chain)
	case "getblocks":
		HandleGetBlocks(request, chain)
	case "getdata":
		HanldeGetData(request, chain)
	case "tx":
		HandleTransaction(request, chain)
	case "version":
		HanleVersion(request, chain)
	default:
		fmt.Printf("Unknown command %s\n", command)
	}
}

// Handle the node address of peer into the pipe network
func HandleAddress(request []byte) {
	var buff bytes.Buffer
	var payload Addr

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	ErrorHandler(err)

	KnownNodes = append(KnownNodes, payload.AddrList...)
	fmt.Printf("There are %d known nodes", len(KnownNodes))
	RequestBlocks()
}

// Handle the add blocks into the chain from a peer into the pipe network
func HandleBlock(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Block

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	ErrorHandler(err)

	blockData := payload.Block
	block := blockchain.Deserialize(blockData)

	fmt.Printf("Recevied a new block")
	chain.AddBlock(block)

	fmt.Printf("Added block %x\n", block.Hash)

	if len(blocksInTransit) > 0 {
		blockHash := blocksInTransit[0]
		SendGetData(payload.AddrFrom, "block", blockHash)
		blocksInTransit = blocksInTransit[1:]
	} else {
		UTXOSet := blockchain.UTXOSet{chain}
		UTXOSet.Reindex()
	}
}

// Handle claim of blocks into the chain from a peer into the pipe network
func HandleGetBlocks(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetBlocks

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	ErrorHandler(err)

	blocks := chain.GetBlockHashes()
	SendInventory(payload.AddrFrom, "block", blocks)
}

// Handle claim of data link into the chain from a peer into the pipe network
func HanldeGetData(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetData

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	ErrorHandler(err)

	switch payload.Type {
	case "block":
		block, err := chain.GetBlock([]byte(payload.ID))
		if err != nil {
			return
		}
		SendBlock(payload.AddrFrom, &block)
	case "tx":
		txID := hex.EncodeToString(payload.ID)
		tx := memoryPool[txID]

		SendTransaction(payload.AddrFrom, &tx)
	}
}

// Handle claim of chain version from a peer into the pipe network
func HanleVersion(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Version

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	ErrorHandler(err)

	bestHeight := chain.GetBestHeight()
	otherHeight := payload.BestHeight

	if bestHeight < otherHeight {
		SendGetBlock(payload.AddrFrom)
	} else {
		SendVersion(payload.AddrFrom, chain)
	}

	if !NodeIsKnown(payload.AddrFrom) {
		KnownNodes = append(KnownNodes, payload.AddrFrom)
	}
}

// Handle add transaction into chain from a peer into the pipe network
func HandleTransaction(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Tx

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	ErrorHandler(err)

	txData := payload.Transaction
	tx := blockchain.DeserializeTransaction(txData)
	memoryPool[hex.EncodeToString(tx.ID)] = tx

	fmt.Printf("%s, %d", nodeAddress, len(memoryPool))

	// check if the node address is the main node
	if nodeAddress == KnownNodes[0] {
		for _, node := range KnownNodes {
			if node != nodeAddress && node != payload.AddrFrom {
				SendInventory(node, "tx", [][]byte{tx.ID})
			}
		}
	} else {
		// apply only for all the miner node
		if len(memoryPool) >= 2 && len(minerAddress) > 0 {
			MineTransaction(chain)
		}
	}
}

// Handle add inventory into chain from a peer into the pipe network
func HandleInventory(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Inventory

	buff.Write(request[commandLength:])
	dec := gob.NewDecoder(&buff)
	err := dec.Decode(&payload)
	ErrorHandler(err)

	fmt.Printf("Recevied inventory with %d %s \n", len(payload.Items), payload.Type)

	switch payload.Type {
	case "block":
		blocksInTransit = payload.Items

		blockHash := payload.Items[0]
		SendGetData(payload.AddrFrom, "block", blockHash)

		newInTransit := [][]byte{}
		for _, block := range blocksInTransit {
			if bytes.Equal(block, blockHash) {
				newInTransit = append(newInTransit, block)
			}
		}
		blocksInTransit = newInTransit
	case "tx":
		txID := payload.Items[0]

		// check if the incomming transcation is in the memory pool, if it's not clain the transaction data
		if memoryPool[hex.EncodeToString(txID)].ID == nil {
			SendGetData(payload.AddrFrom, "tx", txID)
		}
	}

}

// Start the server for a node into the peer of the network
func StartServer(nodeID, minerAddress string) {
	nodeAddress = fmt.Sprintf("localhost:%s", nodeID)
	minerAddress = minerAddress

	// open the TCP stream
	listener, err := net.Listen(protocol, nodeAddress)
	ErrorHandler(err)
	defer listener.Close()

	chain := blockchain.CountinueBlockChain(nodeID)
	defer chain.Database.Close()

	go CloseDB(chain)

	// check if the node address is the centralize node
	if nodeAddress != KnownNodes[0] {
		SendVersion(KnownNodes[0], chain)
	}

	// start loop to maintain the connection
	for {
		conn, err := listener.Accept()
		ErrorHandler(err)
		// open a side thread by connection
		go HandleConnection(conn, chain)
	}
}
