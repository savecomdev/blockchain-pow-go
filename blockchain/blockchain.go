package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dgraph-io/badger"
)

const (
	dbPath = "./tmp/blocks_%s"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

// Check the file link to the DB
func existDB(path string) bool {
	if _, err := os.Stat(path + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}
	return true
}

// Retry on open the DB instance
func retry(dir string, originalOpts badger.Options) (*badger.DB, error) {
	lockPath := filepath.Join(dir, "LOCK")
	if err := os.Remove(lockPath); err != nil {
		return nil, fmt.Errorf(`removing "LOCK": %s`, err)
	}

	retryOpts := originalOpts
	retryOpts.Truncate = true

	db, err := badger.Open(retryOpts)
	return db, err
}

// Open the DB instance
func openDB(dir string, opts badger.Options) (*badger.DB, error) {
	if db, err := badger.Open(opts); err != nil {
		if strings.Contains(err.Error(), "LOCK") {
			if db, err := retry(dir, opts); err != nil {
				log.Println("Database unlocked, value log truncated")
				return db, nil
			}
			log.Println("Could not unlock database:", err)
		}
		return nil, err
	} else {
		return db, nil
	}
}

// Initialization the chain for the address
func InitBlockChain(address, nodeID string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeID)
	if existDB(path) {
		fmt.Println("Blockchain already exists!!!")
		runtime.Goexit()
	}

	var lastHash []byte

	// configure the database
	opts := badger.DefaultOptions(dbPath)

	// open the database
	db, err := openDB(path, opts)
	ErrorHandler(err)

	err = db.Update(func(txn *badger.Txn) error {
		cbtx := CoinBaseTx(address, "First Transaction from Genesis")
		genesis := Genesis(cbtx)
		fmt.Println("Genesis created")

		err = txn.Set(genesis.Hash, genesis.Serialize())
		ErrorHandler(err)

		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err
	})

	ErrorHandler(err)

	blockchain := BlockChain{lastHash, db}

	return &blockchain
}

// Get the  current chain for the address
func CountinueBlockChain(nodeID string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeID)
	if !existDB(path) {
		fmt.Println("No existing Blockchain found, create one !!!")
		runtime.Goexit()
	}

	var lastHash []byte

	// configure the database
	opts := badger.DefaultOptions(dbPath)

	// open the database
	db, err := openDB(path, opts)
	ErrorHandler(err)

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		ErrorHandler(err)
		lastHash, err = item.ValueCopy(lastHash)
		return err
	})
	ErrorHandler(err)

	chain := BlockChain{lastHash, db}

	return &chain
}

// Add a new block into the chain
func (chain *BlockChain) MineBlock(transactions []*Transaction) *Block {
	var lastHash []byte
	var lastHeight int

	for _, tx := range transactions {
		if !chain.VerifyTransaction(tx) {
			log.Panic("Invalid Transaction")
		}
	}

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		lastHash, err = item.ValueCopy(lastHash)
		if err != nil {
			return err
		}

		item, err = txn.Get(lastHash)
		if err != nil {
			return err
		}

		var lastBlockData []byte
		lastBlockData, err = item.ValueCopy(lastBlockData)

		lastBlock := Deserialize(lastBlockData)
		lastHeight = lastBlock.Height

		return err
	})

	ErrorHandler(err)

	// create a new block with the last hash
	newBlock := CreateBlock(transactions, lastHash, lastHeight+1)

	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())
		ErrorHandler(err)

		err = txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash

		return err
	})

	ErrorHandler(err)

	return newBlock
}

func (chain *BlockChain) AddBlock(block *Block) {
	err := chain.Database.Update(func(txn *badger.Txn) error {

		if _, err := txn.Get(block.Hash); err != nil {
			return nil
		}

		blockData := block.Serialize()
		err := txn.Set(block.Hash, blockData)
		if err != nil {
			return err
		}

		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}

		var lastHash []byte
		lastHash, err = item.ValueCopy(lastHash)
		if err != nil {
			return err
		}

		item, err = txn.Get(lastHash)
		if err != nil {
			return err
		}

		var lastBlockData []byte
		lastBlockData, err = item.ValueCopy(lastBlockData)
		if err != nil {
			return err
		}

		lastBlock := Deserialize(lastBlockData)

		if block.Height > lastBlock.Height {
			err = txn.Set([]byte("lh"), block.Hash)
			if err != nil {
				return err
			}
			chain.LastHash = block.Hash
		}

		return nil
	})
	ErrorHandler(err)
}

// Get a block into the chain by the hash value
func (chain *BlockChain) GetBlock(blockHash []byte) (Block, error) {
	var block Block

	// open read only into the DB
	err := chain.Database.View(func(txn *badger.Txn) error {
		if item, err := txn.Get(blockHash); err != nil {
			return errors.New("Block is not found")
		} else {
			var blockdata []byte
			blockdata, err = item.ValueCopy(blockdata)
			if err != nil {
				return err
			}
			block = *Deserialize(blockdata)
		}

		return nil
	})
	if err != nil {
		return block, err
	}

	return block, nil
}

// Get the map with hashes linked to block into the chain
func (chain *BlockChain) GetBlockHashes() [][]byte {
	var blocks [][]byte

	iter := chain.Iterator()

	for {
		block := iter.Next()

		blocks = append(blocks, block.Hash)

		// search the first block without previous to leave the loop
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return blocks
}

// Get the max height index of block into the chain
func (chain *BlockChain) GetBestHeight() int {
	var lastBlock Block

	// open read only into the DB
	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}

		var lastHash []byte
		lastHash, err = item.ValueCopy(lastHash)
		if err != nil {
			return err
		}

		item, err = txn.Get(lastHash)
		if err != nil {
			return err
		}

		var lastBlockData []byte
		lastBlockData, err = item.ValueCopy(lastBlockData)
		lastBlock = *Deserialize(lastBlockData)

		return err
	})
	ErrorHandler(err)

	return lastBlock.Height
}

// Retreive all the output transactions
func (chain *BlockChain) FindUTXO() map[string]TxOutputs {
	UTXO := make(map[string]TxOutputs)

	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}
				outs := UTXO[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXO[txID] = outs
			}
			if !tx.IsCoinbase() {
				for _, in := range tx.Inputs {
					inTxID := hex.EncodeToString(in.ID)
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
				}
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}
	return UTXO
}

// Search a transaction into the chain by the ID
func (chain *BlockChain) FindTransaction(ID []byte) (Transaction, error) {
	iter := chain.Iterator()

	for {
		block := iter.Next()

		for _, tx := range block.Transactions {
			if bytes.Equal(tx.ID, ID) {
				return *tx, nil
			}
		}

		// breaj=k the loop at the last block of the chain
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return Transaction{}, errors.New("Transaction doesn't exist")
}

// Function to sign a transaction into the chain
func (chain *BlockChain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		ErrorHandler(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

// Function to verify a transaction into the chain
func (chain *BlockChain) VerifyTransaction(tx *Transaction) bool {
	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Transaction)

	for _, in := range tx.Inputs {
		prevTX, err := chain.FindTransaction(in.ID)
		ErrorHandler(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
}
