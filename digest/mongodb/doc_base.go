package mongodbstorage

import (
	bsonenc "github.com/ProtoconNet/mitum-currency/v2/digest/util/bson"
	"github.com/ProtoconNet/mitum2/util"
	"github.com/ProtoconNet/mitum2/util/encoder"
	"github.com/ProtoconNet/mitum2/util/hint"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type Doc interface {
	ID() interface{}
}

type BaseDoc struct {
	id          interface{}
	encoderHint hint.Hint
	data        interface{}
	isHinted    bool
}

func NewBaseDoc(id, data interface{}, enc encoder.Encoder) (BaseDoc, error) {
	_, isHinted := data.(hint.Hinter)

	return BaseDoc{
		id:          id,
		encoderHint: enc.Hint(),
		isHinted:    isHinted,
		data:        data,
	}, nil
}

func (do BaseDoc) ID() interface{} {
	return do.id
}

func (do BaseDoc) M() (bson.M, error) {
	m := bson.M{
		"_e":      do.encoderHint.String(),
		"_hinted": do.isHinted,
		"d":       do.data,
	}

	if do.id != nil {
		m["_id"] = do.id
	}

	return m, nil
}

type BaseDocUnmarshaler struct {
	I bson.Raw      `bson:"_id,omitempty"`
	E string        `bson:"_e"`
	D bson.RawValue `bson:"d"`
	H bool          `bson:"_hinted"`
}

func LoadDataFromDoc(b []byte, encs *encoder.Encoders) (bson.Raw /* id */, interface{} /* data */, error) {
	var bd BaseDocUnmarshaler
	if err := bsonenc.Unmarshal(b, &bd); err != nil {
		return nil, nil, err
	}

	ht, err := hint.ParseHint(bd.E)
	if err != nil {
		return nil, nil, err
	}

	enc := encs.Find(ht)
	if enc == nil {
		return nil, nil, util.ErrNotFound.Errorf("encoder not found for %q", bsonenc.BSONEncoderHint)
	}

	if !bd.H {
		return bd.I, bd.D, nil
	}

	doc, ok := bd.D.DocumentOK()
	if !ok {
		return nil, nil, errors.Errorf("hinted should be mongodb Document")
	}

	var data interface{}
	if i, err := enc.Decode([]byte(doc)); err != nil {
		return nil, nil, err
	} else {
		data = i
	}

	return bd.I, data, nil
}
