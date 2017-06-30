* make twilio account (http://twilio.com)
* get at least 2 free tier containers (like aws and google cloud)
* set it up like so:

$ go get github.com/sfreiberg/gotwilio
$ go build -o not-today

copy not-today to INSTANCE-A(lets say aws) and to INSTANCE-B(lets say google cloud)

on INSTANCE-A run:
$ not-today -sid XXXXX -token YYYYY -number ZZZZZ -checks INSTANCE-B-IP:8080/@+123123@+111222,https://google.com/@+123123@+111222,https://yahoo.com@+123123@+111222

on INSTANCE-b run

$ not-today -sid XXXXX -token YYYYY -number ZZZZZ -checks INSTANCE-A-IP:8080/@+123123@+111222


so INSTANCE-A is checking that INSTANCE-B is running, and also the websites you are interested in
and INSTANCE-B is checking if INSTANCE-A is running

