package graft

import (
	"github.com/uber-go/zap"
	"google.golang.org/grpc/grpclog"
)

func init() {
	grpclog.SetLogger(&Logger{})

}

var logger = zap.New(zap.NewTextEncoder())

// Logger for grpc
type Logger struct {
}

func (*Logger) Fatal(args ...interface{}) {

}

func (*Logger) Fatalf(format string, args ...interface{}) {

}

func (*Logger) Fatalln(args ...interface{}) {

}

func (*Logger) Print(args ...interface{}) {
}

func (*Logger) Printf(format string, args ...interface{}) {
}

func (*Logger) Println(args ...interface{}) {}
