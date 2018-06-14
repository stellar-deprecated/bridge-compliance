package bridge

import (
	b "github.com/stellar/go/build"
	"github.com/stellar/go/services/bridge/protocols"
)

// CreateAccountOperationBody represents create_account operation
type CreateAccountOperationBody struct {
	Source          *string
	Destination     string
	StartingBalance string `json:"starting_balance"`
}

// ToTransactionMutator returns go-stellar-base TransactionMutator
func (op CreateAccountOperationBody) ToTransactionMutator() b.TransactionMutator {
	mutators := []interface{}{
		b.Destination{op.Destination},
		b.NativeAmount{op.StartingBalance},
	}

	if op.Source != nil {
		mutators = append(mutators, b.SourceAccount{*op.Source})
	}

	return b.CreateAccount(mutators...)
}

// Validate validates if operation body is valid.
func (op CreateAccountOperationBody) Validate() error {
	if !protocols.IsValidAccountID(op.Destination) {
		return protocols.NewInvalidParameterError("destination", op.Destination, "Destination must be a public key (starting with `G`)")
	}

	if !protocols.IsValidAmount(op.StartingBalance) {
		return protocols.NewInvalidParameterError("starting_balance", op.StartingBalance, "Not a valid amount.")
	}

	if op.Source != nil && !protocols.IsValidAccountID(*op.Source) {
		return protocols.NewInvalidParameterError("source", *op.Source, "Source must be a public key (starting with `G`)")
	}

	return nil
}
