package graft

import "google.golang.org/grpc/grpclog"

func init() {
	grpclog.SetLogger(&Logger{})

}

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
