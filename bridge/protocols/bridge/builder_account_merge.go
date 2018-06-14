package bridge

import (
	b "github.com/stellar/go/build"
	"github.com/stellar/go/services/bridge/protocols"
)

// AccountMergeOperationBody represents account_merge operation
type AccountMergeOperationBody struct {
	Source      *string
	Destination string
}

// ToTransactionMutator returns stellar/go TransactionMutator
func (op AccountMergeOperationBody) ToTransactionMutator() b.TransactionMutator {
	mutators := []interface{}{b.Destination{op.Destination}}

	if op.Source != nil {
		mutators = append(mutators, b.SourceAccount{*op.Source})
	}

	return b.AccountMerge(mutators...)
}

// Validate validates if operation body is valid.
func (op AccountMergeOperationBody) Validate() error {
	if !protocols.IsValidAccountID(op.Destination) {
		return protocols.NewInvalidParameterError("destination", op.Destination, "Destination must start with `G`.")
	}

	if op.Source != nil && !protocols.IsValidAccountID(*op.Source) {
		return protocols.NewInvalidParameterError("source", *op.Source, "Source must start with `G`.")
	}

	return nil
}
