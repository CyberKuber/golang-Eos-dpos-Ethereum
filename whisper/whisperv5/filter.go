
//此源码被清华学神尹成大魔王专业翻译分析并修改
//尹成QQ77025077
//尹成微信18510341407
//尹成所在QQ群721929980
//尹成邮箱 yinc13@mails.tsinghua.edu.cn
//尹成毕业于清华大学,微软区块链领域全球最有价值专家
//https://mvp.microsoft.com/zh-cn/PublicProfile/4033620
//
//
//
//
//
//
//
//
//
//
//
//
//
//
//

package whisperv5

import (
	"crypto/ecdsa"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
)

type Filter struct {
Src        *ecdsa.PublicKey  //
KeyAsym    *ecdsa.PrivateKey //
KeySym     []byte            //
Topics     [][]byte          //
PoW        float64           //
AllowP2P   bool              //
SymKeyHash common.Hash       //

	Messages map[common.Hash]*ReceivedMessage
	mutex    sync.RWMutex
}

type Filters struct {
	watchers map[string]*Filter
	whisper  *Whisper
	mutex    sync.RWMutex
}

func NewFilters(w *Whisper) *Filters {
	return &Filters{
		watchers: make(map[string]*Filter),
		whisper:  w,
	}
}

func (fs *Filters) Install(watcher *Filter) (string, error) {
	if watcher.Messages == nil {
		watcher.Messages = make(map[common.Hash]*ReceivedMessage)
	}

	id, err := GenerateRandomID()
	if err != nil {
		return "", err
	}

	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	if fs.watchers[id] != nil {
		return "", fmt.Errorf("failed to generate unique ID")
	}

	if watcher.expectsSymmetricEncryption() {
		watcher.SymKeyHash = crypto.Keccak256Hash(watcher.KeySym)
	}

	fs.watchers[id] = watcher
	return id, err
}

func (fs *Filters) Uninstall(id string) bool {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	if fs.watchers[id] != nil {
		delete(fs.watchers, id)
		return true
	}
	return false
}

func (fs *Filters) Get(id string) *Filter {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	return fs.watchers[id]
}

func (fs *Filters) NotifyWatchers(env *Envelope, p2pMessage bool) {
	var msg *ReceivedMessage

	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

i := -1 //
	for _, watcher := range fs.watchers {
		i++
		if p2pMessage && !watcher.AllowP2P {
			log.Trace(fmt.Sprintf("msg [%x], filter [%d]: p2p messages are not allowed", env.Hash(), i))
			continue
		}

		var match bool
		if msg != nil {
			match = watcher.MatchMessage(msg)
		} else {
			match = watcher.MatchEnvelope(env)
			if match {
				msg = env.Open(watcher)
				if msg == nil {
					log.Trace("processing message: failed to open", "message", env.Hash().Hex(), "filter", i)
				}
			} else {
				log.Trace("processing message: does not match", "message", env.Hash().Hex(), "filter", i)
			}
		}

		if match && msg != nil {
			log.Trace("processing message: decrypted", "hash", env.Hash().Hex())
			if watcher.Src == nil || IsPubKeyEqual(msg.Src, watcher.Src) {
				watcher.Trigger(msg)
			}
		}
	}
}

func (f *Filter) processEnvelope(env *Envelope) *ReceivedMessage {
	if f.MatchEnvelope(env) {
		msg := env.Open(f)
		if msg != nil {
			return msg
		} else {
			log.Trace("processing envelope: failed to open", "hash", env.Hash().Hex())
		}
	} else {
		log.Trace("processing envelope: does not match", "hash", env.Hash().Hex())
	}
	return nil
}

func (f *Filter) expectsAsymmetricEncryption() bool {
	return f.KeyAsym != nil
}

func (f *Filter) expectsSymmetricEncryption() bool {
	return f.KeySym != nil
}

func (f *Filter) Trigger(msg *ReceivedMessage) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if _, exist := f.Messages[msg.EnvelopeHash]; !exist {
		f.Messages[msg.EnvelopeHash] = msg
	}
}

func (f *Filter) Retrieve() (all []*ReceivedMessage) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	all = make([]*ReceivedMessage, 0, len(f.Messages))
	for _, msg := range f.Messages {
		all = append(all, msg)
	}

f.Messages = make(map[common.Hash]*ReceivedMessage) //
	return all
}

func (f *Filter) MatchMessage(msg *ReceivedMessage) bool {
	if f.PoW > 0 && msg.PoW < f.PoW {
		return false
	}

	if f.expectsAsymmetricEncryption() && msg.isAsymmetricEncryption() {
		return IsPubKeyEqual(&f.KeyAsym.PublicKey, msg.Dst) && f.MatchTopic(msg.Topic)
	} else if f.expectsSymmetricEncryption() && msg.isSymmetricEncryption() {
		return f.SymKeyHash == msg.SymKeyHash && f.MatchTopic(msg.Topic)
	}
	return false
}

func (f *Filter) MatchEnvelope(envelope *Envelope) bool {
	if f.PoW > 0 && envelope.pow < f.PoW {
		return false
	}

	if f.expectsAsymmetricEncryption() && envelope.isAsymmetric() {
		return f.MatchTopic(envelope.Topic)
	} else if f.expectsSymmetricEncryption() && envelope.IsSymmetric() {
		return f.MatchTopic(envelope.Topic)
	}
	return false
}

func (f *Filter) MatchTopic(topic TopicType) bool {
	if len(f.Topics) == 0 {
//
		return true
	}

	for _, bt := range f.Topics {
		if matchSingleTopic(topic, bt) {
			return true
		}
	}
	return false
}

func matchSingleTopic(topic TopicType, bt []byte) bool {
	if len(bt) > TopicLength {
		bt = bt[:TopicLength]
	}

	if len(bt) == 0 {
		return false
	}

	for j, b := range bt {
		if topic[j] != b {
			return false
		}
	}
	return true
}

func IsPubKeyEqual(a, b *ecdsa.PublicKey) bool {
	if !ValidatePublicKey(a) {
		return false
	} else if !ValidatePublicKey(b) {
		return false
	}
//
	return a.X.Cmp(b.X) == 0 && a.Y.Cmp(b.Y) == 0
}
