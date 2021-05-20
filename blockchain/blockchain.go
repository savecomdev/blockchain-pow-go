package blockchain

import (
	"encoding/hex"
	"fmt"
	"os"
	"runtime"

	"github.com/dgraph-io/badger"
)

const (
	dbPath      = "./tmp/blocks"
	dbFile      = "./tmp/blocks/MANIFEST"
	genesisData = "First Transaction from Genesis"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// Initialization the chain for the address
func InitBlockChain(address string) *BlockChain {
	if DBexists() {
		fmt.Println("Blockchain already exists!!!")
		runtime.Goexit()
	}

	var lastHash []byte

	// configure the database
	opts := badger.DefaultOptions(dbPath)

	// open the database
	db, err := badger.Open(opts)
	ErrorHandler(err)

	err = db.Update(func(txn *badger.Txn) error {
		cbtx := CoinBaseTx(address, genesisData)
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
func CountinueBlockChain(address string) *BlockChain {
	if DBexists() == false {
		fmt.Println("No existing Blockchain found, create one !!!")
		runtime.Goexit()
	}

	var lastHash []byte

	// configure the database
	opts := badger.DefaultOptions(dbPath)

	// open the database
	db, err := badger.Open(opts)
	ErrorHandler(err)

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		ErrorHandler(err)

		err = item.Value(func(val []byte) error {
			lastHash = val
			return nil
		})

		return err
	})
	ErrorHandler(err)

	chain := BlockChain{lastHash, db}

	return &chain
}

func (chain *BlockChain) AddBlock(txs []*Transaction) {
	var lastHash []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		ErrorHandler(err)

		err = item.Value(func(val []byte) error {
			lastHash = val
			return nil
		})

		return err
	})

	ErrorHandler(err)

	// create a new block with the last hash
	newBlock := CreateBlock(txs, lastHash)

	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Serialize())
		ErrorHandler(err)
		err = txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash

		return err
	})

	ErrorHandler(err)
}

// Convert the blockchain into iterator
func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}
	return iter
}

func (iter *BlockChainIterator) Next() *Block {
	var block *Block

	err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrentHash)
		ErrorHandler(err)

		err = item.Value(func(encodedBlock []byte) error {
			block = Deserialize(encodedBlock)
			return nil
		})

		return err
	})

	ErrorHandler(err)

	iter.CurrentHash = block.PrevHash

	return block
}

func DBexists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// Retreive all transactions without outputs
func (chain *BlockChain) FindUnspentTransactions(address string) []Transaction {
	var unspentTxs []Transaction

	// create tempory maps for outputs
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	// search loop
	for {
		block := iter.Next()

		for _, tx := range block.Transaction {
			txID := hex.EncodeToString(tx.ID)

		OutputsLoop:
			for outIdx, out := range tx.Outputs {
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue OutputsLoop
						}
					}
				}

				if out.CanBeUnlocked(address) {
					unspentTxs = append(unspentTxs, *tx)
				}
			}

			// case if not the first transaction
			if tx.IsCoinbase() == false {
				for _, in := range tx.Inputs {
					if in.CanUnlock(address) {
						inTxID := hex.EncodeToString(in.ID)
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
					}
				}
			}
		}

		// flag to leave the loop
		if len(block.PrevHash) == 0 {
			break
		}
	}

	return unspentTxs
}

// Retreive all the output transactions
func (chain *BlockChain) FindUTXO(address string) []TxOutput {
	var UTXOs []TxOutput
	unspentTransactions := chain.FindUnspentTransactions(address)

	for _, tx := range unspentTransactions {
		for _, out := range tx.Outputs {
			if out.CanBeUnlocked(address) {
				UTXOs = append(UTXOs, out)
			}
		}
	}

	return UTXOs
}

// Retreive all the available output transaction for an amount
func (chain *BlockChain) FincSpendabaleOutputs(address string, amount int) (int, map[string][]int) {
	unspentOuts := make(map[string][]int)
	unspentTransactions := chain.FindUnspentTransactions(address)
	accumulated := 0

Work:
	for _, tx := range unspentTransactions {
		txID := hex.EncodeToString(tx.ID)

		for outIdx, out := range tx.Outputs {
			// check if the address is good and add enough coins for the amount
			if out.CanBeUnlocked(address) && accumulated < amount {
				accumulated += out.Value
				unspentOuts[txID] = append(unspentOuts[txID], outIdx)

				if accumulated >= amount {
					break Work
				}
			}
		}
	}

	return accumulated, unspentOuts
}
