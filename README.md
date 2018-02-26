#### ctlog-acquisition

NOTE: This code is very much work in progress and should not be used at all until further notice.

A golang application built to assist project Rapid7 - Project Sonar

Problems and TODO:

 - Clean up the code
 - Implement backoff algorithm to retry failed download
 - Authentication option for web server
 - cli arguments for various options.

#### HOW TO:

To see how the code may work, you can try running 
```
go get github.com/GovAuCSU/ctlog-acquisition 
cd $GOPATH/src/github.com/GovAuCSU/ctlog-acquisition/cmd
go run main.go
```

visit http://localhost:3000 to download the populated DNS name file
