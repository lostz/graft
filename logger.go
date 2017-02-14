package graft

import "google.golang.org/grpc/grpclog"

// Logger mock  grpc logger
type Logger struct {
}

// Fatal mock grpc
func (*Logger) Fatal(args ...interface{}) {

}

// Fatalf mock grpc
func (*Logger) Fatalf(format string, args ...interface{}) {

}

// Fatalln mock grpc
func (*Logger) Fatalln(args ...interface{}) {

}

// Print mock grpc
func (*Logger) Print(args ...interface{}) {
}

// Printf mock grpc
func (*Logger) Printf(format string, args ...interface{}) {
}

// Println mock grpc
func (*Logger) Println(args ...interface{}) {}

func init() {
	grpclog.SetLogger(&Logger{})

}
