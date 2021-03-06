/*
	比特币中，没有付款人和收款人，只有输入(input)和输出(output)，
	每个输入都对应着之前别人给你转账时产生的某个输出。
	一笔交易中可以有多个输入和多个输出，给自己找零就是给自己生成一个输出。

		输出产生：
			先从张三给李四转账开始说起，张三给李四转账时，比特币系统会生成一个output，这个output里面包括两个东西：
				1. 转的金额，例如100
				2. 一个锁定脚本，使用李四的**公钥哈希**对转账金额1btc进行锁定，可以理解为用公钥哈希加密了。
			真实的锁定脚本
				锁定脚本：给我收款人的地址，我用这个人公钥进行锁定
				解锁脚本：提供支付人的私钥签名（公钥）

		输入产生：
			与output对应的是input结构，每一个input都源自一个output，在李四对王五进行转账时，系统会创建input，为了定位这笔钱的来源，这个input结构包含以下内容：
				1. 在哪一笔交易中，即需要张三->李四这笔转账的交易ID(hash)
				2. 所引用交易的那个output，所以需要一个output的索引(int)
				3. 定位到了这个output，如何证明能支配呢，所以需要一个张三的签名。（解锁脚本，包括签名和自己的公钥）

		未消费输出（UTXO）：
			1. UTXO：unspent transaction output，是比特币交易中最小的支付单元，不可分割，每一个UTXO必须一次性消耗完，然后生成新的UTXO，存放在比特币网络的UTXO池中。
			2. UTXO是不能再分割、被所有者锁住或记录于区块链中的并被整个网络识别成货币单位的一定量的比特币货币。
			3. 比特币网络监测着以百万为单位的所有可用的（未花费的）UTXO。当一个用户接收比特币时，金额被当作UTXO记录到区块链里。这样，一个用户的比特币会被当作UTXO分散到数百个交易和数百个区块中。
			4. 实际上，并不存在储存比特币地址或账户余额的地点，只有被所有者锁住的、分散的UTXO。
			5. "一个用户的比特币余额"，这个概念是一个通过比特币钱包应用创建的派生之物。比特币钱包通过扫描区块链并聚合所有属于该用户的UTXO来计算该用户的余额。
			6. UTXO被每一个全节点比特币客户端在一个储存于内存中的数据库所追踪，该数据库也被称为“UTXO集”或者"UTXO池"。新的交易从UTXO集中消耗（支付）一个或多个输出。

*/

package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"math/big"
	"strings"
	"time"
)

//Transaction 交易
type Transaction struct {
	TXID      []byte     //交易ID
	TXInputs  []TXInput  //交易输入(N个)
	TXOutputs []TXOutput //交易输出（N个）
	TimeStamp uint64     //创建交易的时间
}

//TXInput 交易输入：指明交易发起人可支付资金的来源
type TXInput struct {
	TXID       []byte //引用output所在交易的ID
	Index      int64  //引用output在output集合中的索引值
	ScriptSign []byte //解锁脚本：付款人对当前新交易的签名
	PubKey     []byte //解锁脚本：付款人的公钥
}

//TXOutput 交易输出：包含资金接收方的相关信息，作为下一个交易的输入
type TXOutput struct {
	Value            float64 //转账金额
	ScriptPubKeyHash []byte  //锁定脚本：收款人的公钥哈希（地址）
}

//NewTXOutput 创建一个人output
func NewTXOutput(address string, amount float64) TXOutput {
	output := TXOutput{
		Value: amount,
	}
	//通过地址获取公钥哈希
	pubKeyHash := GetPubKeyHashFromAddress(address)
	output.ScriptPubKeyHash = pubKeyHash
	return output
}

//获取交易ID：计算交易哈希
func (tx *Transaction) setHash() error {
	//对tx进行gob编码获得字节流，然后计算sha256，赋值给TXID
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(tx)
	if err != nil {
		fmt.Println(err)
		return err
	}
	hash := sha256.Sum256(buffer.Bytes())
	tx.TXID = hash[:]
	return nil
}

//挖矿奖励
var reward = 12.5

//NewCoinbaseTX 创建挖矿交易(没有input因此不需要签名，只有一个output获得挖矿奖励)
func NewCoinbaseTX(miner /*矿工*/ string, data string) *Transaction {
	input := TXInput{TXID: nil, Index: -1, ScriptSign: nil, PubKey: []byte(data)} //挖矿不需要签名，由矿工任意填写
	output := NewTXOutput(miner, reward)
	timStamp := time.Now().Unix()

	tx := Transaction{
		TXID:      nil,
		TXInputs:  []TXInput{input},
		TXOutputs: []TXOutput{output},
		TimeStamp: uint64(timStamp),
	}
	tx.setHash()
	return &tx
}

//NewTransaction 创建普通交易
//from - 付款人，to - 收款人， amount - 转账金额
func NewTransaction(from string, to string, amount float64, bc *BlockChain) *Transaction {

	//钱包在此使用：from -> 钱包 -> 私钥 -> 签名
	//打开钱包
	wm := NewWalletManager()
	if wm == nil {
		fmt.Println("打开钱包失败")
		return nil
	}
	//找到对应的钱包
	wallet, ok := wm.Wallets[from]
	if !ok {
		fmt.Println("未找到付款人地址对应的私钥")
		return nil
	}
	priKey := wallet.PrivateKey                      //签名使用
	pubKey := wallet.PublicKey                       //获得公钥
	pubKeyHash := GetPubKeyHashFromPublicKey(pubKey) //获得公钥哈希

	//遍历账本，找到满足条件的utxo集合，返回utxo集合的总金额
	var spentUTXO = make(map[string][]int64) //将要使用的uxto集合
	var retValue float64                     //utxo的总金额

	//遍历账本，找到from能使用的utxo集合及包含的所有金额
	spentUTXO, retValue = bc.findNeedUTXO(pubKeyHash, amount)
	//金额不足
	if retValue < amount {
		fmt.Println("金额不足，创建交易失败")
		return nil
	}

	var inputs []TXInput
	var outputs []TXOutput
	//拼接inputs
	//遍历utxo集合，把每个putput转为input
	for txid, indexArray := range spentUTXO {
		//遍历获取output的下标值
		for _, i := range indexArray {
			input := TXInput{
				TXID:       []byte(txid),
				Index:      i,
				ScriptSign: nil,
				PubKey:     pubKey,
			}
			inputs = append(inputs, input)
		}

	}

	//拼接outputs
	//创建一个属于to的output
	output1 := NewTXOutput(to, amount)
	outputs = append(outputs, output1)
	if retValue > amount {
		//如果总金额大于转账金额，找零：给from创建一个output
		output2 := NewTXOutput(from, retValue-amount)
		outputs = append(outputs, output2)
	}

	timeStamp := time.Now().Unix()
	//计算哈希值，返回
	tx := Transaction{nil, inputs, outputs, uint64(timeStamp)}
	tx.setHash()

	//交易签名
	if !bc.SignTransaction(&tx, priKey) {
		fmt.Println("交易签名失败")
		return nil
	}

	return &tx
}

//判断交易是否为挖矿交易
func (tx *Transaction) isCoinBaseTX() bool {
	inputs := tx.TXInputs
	//挖矿交易：input个数为1,ID为nil,索引为-1
	if len(inputs) == 1 && inputs[0].TXID == nil && inputs[0].Index == -1 {
		return true
	}
	return false
}

//Sign 实际签名动作(私钥，inputs所引用的output所在交易的集合：key:交易ID,value:交易本身)
func (tx *Transaction) Sign(priKey *ecdsa.PrivateKey, prevTXs map[string]*Transaction) bool {

	//挖矿交易不需要签名
	if tx.isCoinBaseTX() {
		return true
	}

	//获取交易副本，置空pubKey和Sign
	txCopy := tx.trimmedCopy()
	//遍历inputs
	for i, input := range txCopy.TXInputs {
		prevTX := prevTXs[string(input.TXID)]
		if prevTX == nil {
			return false
		}
		//input引用的output
		output := prevTX.TXOutputs[input.Index]
		//获取引用的output公钥哈希
		txCopy.TXInputs[i].PubKey = output.ScriptPubKeyHash
		//对交易副本进行签名
		txCopy.setHash() //计算交易哈希

		//将input的pubKey字段置空
		txCopy.TXInputs[i].PubKey = nil //还原数据，防止干扰后面的input签名

		hashData := txCopy.TXID //要签名的数据
		//签名
		r, s, err := ecdsa.Sign(rand.Reader, priKey, hashData)
		if err != nil {
			fmt.Println("签名失败")
			return false
		}
		signature := append(r.Bytes(), s.Bytes()...)
		//将数字签名赋值给原始交易
		tx.TXInputs[i].ScriptSign = signature
	}

	fmt.Println("交易签名成功")
	return true
}

//创建一个交易副本：每个input的pubKey和Sign都置空
func (tx *Transaction) trimmedCopy() *Transaction {
	var inputs []TXInput
	var outputs []TXOutput

	//每个input的pubKey和Sign都置空
	for _, input := range tx.TXInputs {
		input := TXInput{
			TXID:       input.TXID,
			Index:      input.Index,
			ScriptSign: nil,
			PubKey:     nil,
		}
		inputs = append(inputs, input)
	}

	outputs = tx.TXOutputs

	txCopy := Transaction{
		tx.TXID,
		inputs,
		outputs,
		tx.TimeStamp,
	}

	return &txCopy
}

//Verify 校验交易签名实际动作
func (tx *Transaction) Verify(prevTXs map[string]*Transaction) bool {

	//挖矿交易不需要签名
	if tx.isCoinBaseTX() {
		return true
	}

	//获取交易副本，置空pubKey和Sign
	txCopy := tx.trimmedCopy()
	//遍历inputs
	for i, input := range tx.TXInputs {
		prevTX := prevTXs[string(input.TXID)]
		if prevTX == nil {
			return false
		}
		//还原数据：得到引用  获取交易哈希值
		output := prevTX.TXOutputs[input.Index]
		txCopy.TXInputs[i].PubKey = output.ScriptPubKeyHash
		txCopy.setHash() //计算交易哈希

		//将input的pubKey字段置空
		txCopy.TXInputs[i].PubKey = nil

		hashData := txCopy.TXID       //要还原的签名的数据
		signature := input.ScriptSign //签名
		pubKey := input.PubKey        //公钥字节流

		//开始校验
		var r, s, x, y big.Int

		//把r和s从签名中截取出来
		r.SetBytes(signature[:len(signature)/2])
		s.SetBytes(signature[len(signature)/2:])

		//把x和y从pubKey中截取出来，还原公钥本身
		x.SetBytes(pubKey[:len(pubKey)/2])
		y.SetBytes(pubKey[len(pubKey)/2:])

		curve := elliptic.P256()
		publicKey := ecdsa.PublicKey{Curve: curve, X: &x, Y: &y}

		//校验
		res := ecdsa.Verify(&publicKey, hashData, &r, &s)
		if !res {
			fmt.Println("签名校验失败")
			return false
		}

	}

	fmt.Println("签名校验成功")
	return true
}

//String方法
func (tx *Transaction) String() string {
	var lines []string

	lines = append(lines, fmt.Sprintf("Transaction %x:", tx.TXID))

	for i, input := range tx.TXInputs {

		lines = append(lines, fmt.Sprintf("Input %d:", i))
		lines = append(lines, fmt.Sprintf("TXID: %x", input.TXID))
		lines = append(lines, fmt.Sprintf("Out: %d", input.Index))
		lines = append(lines, fmt.Sprintf("Signature: %x", input.ScriptSign))
		lines = append(lines, fmt.Sprintf("PubKey: %x", input.PubKey))
	}

	for i, output := range tx.TXOutputs {
		lines = append(lines, fmt.Sprintf("Output %d:", i))
		lines = append(lines, fmt.Sprintf("Value: %f", output.Value))
		lines = append(lines, fmt.Sprintf("Script: %x", output.ScriptPubKeyHash))
	}

	return strings.Join(lines, "\n")
}
