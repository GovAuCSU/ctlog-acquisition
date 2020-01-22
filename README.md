#### ctlog-acquisition

NOTE: This code is a work in progress and should not be used in production.  Pull requests and issues are most welcome.

A golang application to pull Certificate Transparency logs

Problems and TODO:

 - Clean up the code
 - Implement backoff algorithm to retry failed download
 - Authentication option for web server
 - cli arguments for various options.

#### HOW TO:

To get the code running quickly, try the docker container.  This will start writing the CT logs to a local directory called 'ct_logs'. NOTE - it takes a while before writing any data to the created files.

```
docker run -it --rm --v /ct_logs:/static 2ajpekr8/ctlog-acquisition -disable-webserver -start-current
```

or to build it yourself!

```
docker build . -t go-ctlog
docker run -it --rm --v /ct_logs:/static go-ctlog -disable-webserver -start-current
```

To see how the code may work, you can try running 
```
go get github.com/GovAuCSU/ctlog-acquisition 
cd $GOPATH/src/github.com/GovAuCSU/ctlog-acquisition/cmd
go run main.go
```

visit http://localhost:3000 to download the populated DNS name file
