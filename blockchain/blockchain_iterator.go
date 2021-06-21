package blockchain

import "github.com/dgraph-io/badger"

type BlockChainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

// Convert the blockchain into iterator
func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}
	return iter
}

// Move to the next element into the chain
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
