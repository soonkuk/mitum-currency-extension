package currency

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/spikeekips/mitum-currency/currency"
	"github.com/spikeekips/mitum/base"
	"github.com/spikeekips/mitum/isaac"
	"github.com/spikeekips/mitum/util"
)

var currencyRegisterProcessorPool = sync.Pool{
	New: func() interface{} {
		return new(CurrencyRegisterProcessor)
	},
}

func (CurrencyRegister) Process(
	ctx context.Context, getStateFunc base.GetStateFunc,
) ([]base.StateMergeValue, base.OperationProcessReasonError, error) {
	// NOTE Process is nil func
	return nil, nil, nil
}

type CurrencyRegisterProcessor struct {
	*base.BaseOperationProcessor
	suffrage  base.Suffrage
	threshold base.Threshold
}

func NewCurrencyRegisterProcessor(threshold base.Threshold) GetNewProcessor {
	return func(height base.Height,
		getStateFunc base.GetStateFunc,
		newPreProcessConstraintFunc base.NewOperationProcessorProcessFunc,
		newProcessConstraintFunc base.NewOperationProcessorProcessFunc,
	) (base.OperationProcessor, error) {
		e := util.StringErrorFunc("failed to create new CurrencyRegisterProcessor")

		nopp := currencyRegisterProcessorPool.Get()
		opp, ok := nopp.(*CurrencyRegisterProcessor)
		if !ok {
			return nil, e(nil, "expected CurrencyRegisterProcessor, not %T", nopp)
		}

		b, err := base.NewBaseOperationProcessor(
			height, getStateFunc, newPreProcessConstraintFunc, newProcessConstraintFunc)
		if err != nil {
			return nil, e(err, "")
		}

		opp.BaseOperationProcessor = b
		opp.threshold = threshold

		switch i, found, err := getStateFunc(isaac.SuffrageStateKey); {
		case err != nil:
			return nil, e(err, "")
		case !found, i == nil:
			return nil, e(isaac.ErrStopProcessingRetry.Errorf("empty state"), "")
		default:
			sufstv := i.Value().(base.SuffrageNodesStateValue) //nolint:forcetypeassert //...

			suf, err := sufstv.Suffrage()
			if err != nil {
				return nil, e(isaac.ErrStopProcessingRetry.Errorf("failed to get suffrage from state"), "")
			}

			opp.suffrage = suf
		}

		return opp, nil
	}
}

func (opp *CurrencyRegisterProcessor) PreProcess(
	ctx context.Context, op base.Operation, getStateFunc base.GetStateFunc,
) (context.Context, base.OperationProcessReasonError, error) {
	nop, ok := op.(CurrencyRegister)
	if !ok {
		return ctx, nil, errors.Errorf("expected CurrencyRegister, not %T", op)
	}

	fact, ok := op.Fact().(CurrencyRegisterFact)
	if !ok {
		return ctx, nil, errors.Errorf("expected CurrencyRegisterFact, not %T", op.Fact())
	}

	if err := base.CheckFactSignsBySuffrage(opp.suffrage, opp.threshold, nop.NodeSigns()); err != nil {
		return ctx, base.NewBaseOperationProcessReasonError("not enough signs: %w", err), nil
	}

	item := fact.currency

	if err := checkNotExistsState(StateKeyCurrencyDesign(item.amount.Currency()), getStateFunc); err != nil {
		return ctx, base.NewBaseOperationProcessReasonError("currency design already exists, %q: %w", item.amount.Currency(), err), nil
	}

	if err := checkExistsState(currency.StateKeyAccount(item.genesisAccount), getStateFunc); err != nil {
		return ctx, base.NewBaseOperationProcessReasonError("genesis account not found, %q: %w", item.genesisAccount, err), nil
	}

	if err := checkNotExistsState(StateKeyContractAccount(item.genesisAccount), getStateFunc); err != nil {
		return ctx, base.NewBaseOperationProcessReasonError("contract account cannot be genesis account of currency, %q: %w", item.genesisAccount, err), nil
	}

	if receiver := item.Policy().Feeer().Receiver(); receiver != nil {
		if err := checkExistsState(currency.StateKeyAccount(receiver), getStateFunc); err != nil {
			return ctx, base.NewBaseOperationProcessReasonError("feeer receiver not found, %q: %w", receiver, err), nil
		}

		if err := checkNotExistsState(StateKeyContractAccount(receiver), getStateFunc); err != nil {
			return ctx, base.NewBaseOperationProcessReasonError("contract account cannot be fee receiver, %q: %w", receiver, err), nil
		}
	}

	if err := checkNotExistsState(currency.StateKeyBalance(item.genesisAccount, item.amount.Currency()), getStateFunc); err != nil {
		return ctx, base.NewBaseOperationProcessReasonError("account balance already exists, %q: %w", currency.StateKeyBalance(item.genesisAccount, item.amount.Currency()), err), nil
	}

	return ctx, nil, nil
}

func (opp *CurrencyRegisterProcessor) Process(
	ctx context.Context, op base.Operation, getStateFunc base.GetStateFunc) (
	[]base.StateMergeValue, base.OperationProcessReasonError, error,
) {
	fact, ok := op.Fact().(CurrencyRegisterFact)
	if !ok {
		return nil, nil, errors.Errorf("expected CurrencyRegisterFact, not %T", op.Fact())
	}

	sts := make([]base.StateMergeValue, 4)

	item := fact.currency

	ba := currency.NewBalanceStateValue(item.amount)
	sts[0] = currency.NewBalanceStateMergeValue(
		currency.StateKeyBalance(item.genesisAccount, item.amount.Currency()),
		ba,
	)

	de := NewCurrencyDesignStateValue(item)
	sts[1] = NewCurrencyDesignStateMergeValue(StateKeyCurrencyDesign(item.amount.Currency()), de)

	{
		l, err := createZeroAccount(item.amount.Currency(), getStateFunc)
		if err != nil {
			return nil, nil, base.NewBaseOperationProcessReasonError("failed to create zero account, %q: %w", item.amount.Currency(), err)
		}
		sts[2], sts[3] = l[0], l[1]
	}

	return sts, nil, nil
}

func (opp *CurrencyRegisterProcessor) Close() error {
	opp.suffrage = nil
	opp.threshold = 0

	currencyRegisterProcessorPool.Put(opp)

	return nil
}
