package s3imageserver

import (
	"encoding/json"
	"reflect"

	"github.com/pkg/errors"
)

type ImageSource interface {
	GetImage(string) ([]byte, error)
}

type SourceMap struct {
	//a function that takes a struct and returns an interface of type ImageSource
	sources map[string]*concreteImageSource
}

type concreteImageSource struct {
	inter  interface{}
	val    reflect.Value
	inType reflect.Type
}

func (sm *SourceMap) AddSource(name string, source interface{}) error {
	stringErr := "source must be func(type)ImageSource, "
	sourceVal := reflect.ValueOf(source)
	sourceTyp := sourceVal.Type()

	if sourceTyp.Kind() != reflect.Func {
		return errors.New(stringErr + "expecting a function")
	}

	if sourceTyp.NumIn() != 1 {
		return errors.New(stringErr + "expecting a function with one parameter")
	}

	if sourceTyp.NumOut() != 1 {
		return errors.New(stringErr + "expecting a function with one return value")
	}

	if !sourceTyp.Out(0).Implements(reflect.TypeOf((*ImageSource)(nil)).Elem()) {
		return errors.New(stringErr + "expecting a function with one return value that implements ImageSource")
	}

	if sm.sources == nil {
		sm.sources = make(map[string]*concreteImageSource)
	}

	sm.sources[name] = &concreteImageSource{
		inter:  source,
		val:    sourceVal,
		inType: sourceTyp.In(0),
	}
	return nil
}

func (sm *SourceMap) GetSource(name string, configString json.RawMessage) (ImageSource, error) {
	var imgSource *concreteImageSource
	var ok bool
	if imgSource, ok = sm.sources[name]; !ok {
		return nil, errors.New("Source " + name + " not registered")
	}
	configVal := reflect.New(imgSource.inType)
	config := configVal.Interface()
	err := json.Unmarshal(configString, config)
	if err != nil {
		return nil, errors.Wrapf(err, "Config is not valid for type %v", imgSource.inType)
	}
	retVals := imgSource.val.Call([]reflect.Value{reflect.Indirect(configVal)})

	return retVals[0].Interface().(ImageSource), nil
}
