package walletrpc

import (
	"encoding/hex"
	"fmt"
	"github.com/Qitmeer/qitmeer-lib/core/address"
	"github.com/Qitmeer/qitmeer-lib/crypto/ecc/secp256k1"
	"github.com/Qitmeer/qitmeer-lib/params"
	util "github.com/Qitmeer/qitmeer-wallet/utils"
	"github.com/Qitmeer/qitmeer-wallet/wallet/txrules"
	"time"

	//"github.com/Qitmeer/qitmeer-wallet/wallet/txrules"

	//"bytes"
	//"encoding/base64"
	//"encoding/hex"
	//"encoding/json"
	//"errors"
	//"fmt"
	"github.com/Qitmeer/qitmeer-wallet/json/qitmeerjson"
	waddrmgr "github.com/Qitmeer/qitmeer-wallet/waddrmgs"
	"github.com/Qitmeer/qitmeer-lib/core/types"
	"github.com/Qitmeer/qitmeer-lib/engine/txscript"
	//"sync"
	//"time"

	"github.com/Qitmeer/qitmeer-wallet/wallet"
)

// confirmed checks whether a transaction at height txHeight has met minconf
// confirmations for a blockchain at height curHeight.
func Confirmed(minconf, txHeight, curHeight int32) bool {
	return Confirms(txHeight, curHeight) >= minconf
}

// confirms returns the number of confirmations for a transaction in a block at
// height txHeight (or -1 for an unconfirmed tx) given the chain height
// curHeight.
func Confirms(txHeight, curHeight int32) int32 {
	switch {
	case txHeight == -1, txHeight > curHeight:
		return 0
	default:
		return curHeight - txHeight + 1
	}
}

// requestHandler is a handler function to handle an unmarshaled and parsed
// request into a marshalable response.  If the error is a *btcjson.RPCError
// or any of the above special error classes, the server will respond with
// the JSON-RPC appropiate error code.  All other errors use the wallet
// catch-all error code, btcjson.ErrRPCWallet.
type requestHandler func(interface{}, *wallet.Wallet) (interface{}, error)

// createNewAccount handles a createnewaccount request by creating and
// returning a new account. If the last account has no transaction history
// as per BIP 0044 a new account cannot be created so an error will be returned.
func CreateNewAccount(icmd interface{}, w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.CreateNewAccountCmd)

	// The wildcard * is reserved by the rpc server with the special meaning
	// of "all accounts", so disallow naming accounts to this string.
	if cmd.Account == "*" {
		return nil, &qitmeerjson.ErrReservedAccountName
	}

	_, err := w.NextAccount(waddrmgr.KeyScopeBIP0044, cmd.Account)
	if waddrmgr.IsError(err, waddrmgr.ErrLocked) {
		return nil, &qitmeerjson.RPCError{
			Code: qitmeerjson.ErrRPCWalletUnlockNeeded,
			Message: "Creating an account requires the wallet to be unlocked. " +
				"Enter the wallet passphrase with walletpassphrase to unlock",
		}
	}
	return "succ", err
}

// listAccounts handles a listaccounts request by returning a map of account
// names to their balances.
func ListAccounts(icmd interface{}, w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.ListAccountsCmd)

	accountBalances := map[string]float64{}
	results, err := w.AccountBalances(waddrmgr.KeyScopeBIP0044, int32(*cmd.MinConf))
	if err != nil {
		return nil, err
	}
	fmt.Println("results:",results)
	for _, result := range results {
		accountBalances[result.AccountName] = result.AccountBalance.ToCoin()
	}
	// Return the map.  This will be marshaled into a JSON object.
	return accountBalances, nil
}
// getNewAddress handles a getnewaddress request by returning a new
// address for an account.  If the account does not exist an appropiate
// error is returned.
// TODO: Follow BIP 0044 and warn if number of unused addresses exceeds
// the gap limit.
func GetNewAddress(icmd interface{}, w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.GetNewAddressCmd)

	acctName := "default"
	if cmd.Account != nil {
		acctName = *cmd.Account
	}
	if acctName=="imported"{
		return nil,fmt.Errorf("Import account cannot create subaddress.")
	}
	account, err := w.AccountNumber(waddrmgr.KeyScopeBIP0044, acctName)
	if err != nil {
		return nil, err
	}
	addr, err := w.NewAddress( waddrmgr.KeyScopeBIP0044,account)
	if err != nil {
		return nil, err
	}
	// Return the new payment address string.
	return addr.Encode(), nil
}
// getAddressesByAccount handles a getaddressesbyaccount request by returning
// all addresses for an account, or an error if the requested account does
// not exist.
func GetAddressesByAccount(icmd interface{}, w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.GetAddressesByAccountCmd)

	account, err := w.AccountNumber(waddrmgr.KeyScopeBIP0044, cmd.Account)
	if err != nil {
		return nil, err
	}

	addrs, err := w.AccountAddresses(account)
	if err != nil {
		return nil, err
	}

	addrStrs := make([]string, len(addrs))
	for i, a := range addrs {
		addrStrs[i] = a.Encode()
	}
	return addrStrs, nil
}

func GetAccountAndAddress( w *wallet.Wallet,
	minconf int32) (interface{}, error)   {
	a,err:=w.GetAccountAndAddress(waddrmgr.KeyScopeBIP0044,minconf)
	if err != nil {
		return nil, err
	}
	return a,nil
}

// getAccount handles a getaccount request by returning the account name
// associated with a single address.
func GetAccountByAddress(icmd interface{}, w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.GetAccountCmd)

	addr, err := address.DecodeAddress(cmd.Address)
	if err != nil {
		return nil, err
	}

	// Fetch the associated account
	account, err := w.AccountOfAddress(addr)
	if err != nil {
		return nil, &qitmeerjson.ErrAddressNotInWallet
	}

	acctName, err := w.AccountName(waddrmgr.KeyScopeBIP0044, account)
	if err != nil {
		return nil, &qitmeerjson.ErrAccountNameNotFound
	}
	return acctName, nil
}
// dumpPrivKey handles a dumpprivkey request with the private key
// for a single address, or an appropiate error if the wallet
// is locked.
func DumpPrivKey(icmd interface{}, w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.DumpPrivKeyCmd)

	addr, err :=address.DecodeAddress(cmd.Address)
	if err != nil {
		return nil, err
	}

	key, err := w.DumpWIFPrivateKey(addr)
	if waddrmgr.IsError(err, waddrmgr.ErrLocked) {
		// Address was found, but the private key isn't
		// accessible.
		return nil, &qitmeerjson.ErrWalletUnlockNeeded
	}
	return key, err
}

// importPrivKey handles an importprivkey request by parsing
// a WIF-encoded private key and adding it to an account.
func ImportWifPrivKey(icmd interface{}, w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.ImportPrivKeyCmd)

	// Ensure that private keys are only imported to the correct account.
	//
	// Yes, Label is the account name.
	if cmd.Label != nil && *cmd.Label != waddrmgr.ImportedAddrAccountName {
		return nil, &qitmeerjson.ErrNotImportedAccount
	}

	wif, err := util.DecodeWIF(cmd.PrivKey,w.ChainParams())
	if err != nil {
		return nil, &qitmeerjson.RPCError{
			Code:    qitmeerjson.ErrRPCInvalidAddressOrKey,
			Message: "WIF decode failed: " + err.Error(),
		}
	}
	if !wif.IsForNet(w.ChainParams()) {
		return nil, &qitmeerjson.RPCError{
			Code:    qitmeerjson.ErrRPCInvalidAddressOrKey,
			Message: "Key is not intended for " + w.ChainParams().Name,
		}
	}

	// Import the private key, handling any errors.
	_, err = w.ImportPrivateKey(waddrmgr.KeyScopeBIP0044, wif, nil, *cmd.Rescan)
	switch {
	case waddrmgr.IsError(err, waddrmgr.ErrDuplicateAddress):
		// Do not return duplicate key errors to the client.
		return nil, nil
	case waddrmgr.IsError(err, waddrmgr.ErrLocked):
		return nil, &qitmeerjson.ErrWalletUnlockNeeded
	}

	return nil, err
}
func ImportPrivKey(icmd interface{}, w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.ImportPrivKeyCmd)
	// Ensure that private keys are only imported to the correct account.
	//
	// Yes, Label is the account name.
	if cmd.Label != nil && *cmd.Label != waddrmgr.ImportedAddrAccountName {
		return nil, &qitmeerjson.ErrNotImportedAccount
	}

	prihash,err:=hex.DecodeString(cmd.PrivKey)
	if(err!=nil){
		return nil,err
	}
	pri,_:=secp256k1.PrivKeyFromBytes(prihash)
	wif,err:=util.NewWIF(pri,w.ChainParams(),true)
	if err != nil {
		return nil, &qitmeerjson.RPCError{
			Code:    qitmeerjson.ErrRPCInvalidAddressOrKey,
			Message: "private key decode failed: " + err.Error(),
		}
	}
	if !wif.IsForNet(w.ChainParams()) {
		return nil, &qitmeerjson.RPCError{
			Code:    qitmeerjson.ErrRPCInvalidAddressOrKey,
			Message: "Key is not intended for " + w.ChainParams().Name,
		}
	}

	// Import the private key, handling any errors.
	_, err = w.ImportPrivateKey(waddrmgr.KeyScopeBIP0044, wif, nil, *cmd.Rescan)
	switch {
	case waddrmgr.IsError(err, waddrmgr.ErrDuplicateAddress):
		// Do not return duplicate key errors to the client.
		return nil, nil
	case waddrmgr.IsError(err, waddrmgr.ErrLocked):
		return nil, &qitmeerjson.ErrWalletUnlockNeeded
	}

	return "ok", err
}
//sendToAddress handles a sendtoaddress RPC request by creating a new
//transaction spending unspent transaction outputs for a wallet to another
//payment address.  Leftover inputs not sent to the payment address or a fee
//for the miner are sent back to a new address in the wallet.  Upon success,
//the TxID for the created transaction is returned.
func SendToAddress(icmd interface{}, w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.SendToAddressCmd)

	// Transaction comments are not yet supported.  Error instead of
	// pretending to save them.
	if !isNilOrEmpty(cmd.Comment) || !isNilOrEmpty(cmd.CommentTo) {
		return nil, &qitmeerjson.RPCError{
			Code:    qitmeerjson.ErrRPCUnimplemented,
			Message: "Transaction comments are not yet supported",
		}
	}

	amt, err := types.NewAmount(cmd.Amount)
	if err != nil {
		return nil, err
	}

	// Check that signed integer parameters are positive.
	if amt < 0 {
		return nil, qitmeerjson.ErrNeedPositiveAmount
	}

	// Mock up map of address and amount pairs.
	pairs := map[string]types.Amount{
		cmd.Address: amt,
	}

	// sendtoaddress always spends from the default account, this matches bitcoind
	return sendPairs(w, pairs, waddrmgr.DefaultAccountNum, 1, txrules.DefaultRelayFeePerKb)
}

func Updateblock(icmd interface{},w *wallet.Wallet) error {
	cmd := icmd.(*qitmeerjson.UpdateBlockToCmd)
	err:=w.Updateblock(cmd.Toheight)
	if(err!=nil){
		fmt.Println("Updateblock err :",err.Error())
		return err
	}
	return nil
}
func Getbalance(icmd interface{},w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.GetBalanceByAddressCmd)
	m,err:=w.GetBalance(cmd.Address,int32(cmd.MinConf))
	if(err!=nil){
		fmt.Println("getbalance err :",err.Error())
		return nil,err
	}
	return m, nil
}

func Unlock(password string,w *wallet.Wallet) ( error) {
	return w.Unlock([]byte(password),time.After(10*time.Minute))
}

func Getlisttxbyaddr(icmd interface{},w *wallet.Wallet) (interface{}, error) {
	cmd := icmd.(*qitmeerjson.GetListTxByAddrCmd)
	m,err:=w.GetListTxByAddr(cmd.Address,cmd.Stype,cmd.Page,cmd.PageSize)
	if(err!=nil){
		fmt.Println("getListTxByAddr err :",err.Error())
		return nil,err
	}
	return m, nil
}

//sendPairs creates and sends payment transactions.
//It returns the transaction hash in string format upon success
//All errors are returned in btcjson.RPCError format
func sendPairs(w *wallet.Wallet, amounts map[string]types.Amount,
	account uint32, minconf int32, feeSatPerKb types.Amount) (string, error) {

	outputs, err := makeOutputs(amounts, w.ChainParams())
	if err != nil {
		return "", err
	}
	tx, err := w.SendOutputs(outputs, account, minconf, feeSatPerKb)
	if err != nil {
		if err == txrules.ErrAmountNegative {
			return "", qitmeerjson.ErrNeedPositiveAmount
		}
		if waddrmgr.IsError(err, waddrmgr.ErrLocked) {
			return "", &qitmeerjson.ErrWalletUnlockNeeded
		}
		switch err.(type) {
		case qitmeerjson.RPCError:
			return "", err
		}

		return "", &qitmeerjson.RPCError{
			Code:    qitmeerjson.ErrRPCInternal.Code,
			Message: err.Error(),
		}
	}
	return *tx, nil
}

// makeOutputs creates a slice of transaction outputs from a pair of address
// strings to amounts.  This is used to create the outputs to include in newly
// created transactions from a JSON object describing the output destinations
// and amounts.
func makeOutputs(pairs map[string]types.Amount, chainParams *params.Params) ([]*types.TxOutput, error) {
	outputs := make([]*types.TxOutput, 0, len(pairs))
	for addrStr, amt := range pairs {
		addr, err := address.DecodeAddress(addrStr)
		if err != nil {
			return nil, fmt.Errorf("cannot decode address: %s", err)
		}

		pkScript, err := txscript.PayToAddrScript(addr)
		if err != nil {
			return nil, fmt.Errorf("cannot create txout script: %s", err)
		}

		outputs = append(outputs, types.NewTxOutput(uint64(amt), pkScript))
	}
	return outputs, nil
}

func isNilOrEmpty(s *string) bool {
	return s == nil || *s == ""
}