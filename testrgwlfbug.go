// go / swift.

package main

import (
	"github.com/ncw/swift"
	"fmt"
	"io"
	"os"
	"flag"
	"strconv"
	"log"
	"bytes"
	"io/ioutil"
	"sync"
//	"errors"
	"net/http"
	"net/http/httputil"
)

var Dflag *bool
var vflag *bool
var st_user string
var st_key string
var st_auth string
var st_auth_version *int

var initialize_once sync.Once
var cleanup_pan_egg sync.Once
var logfile_fp *os.File
var trace_log bytes.Buffer
var saved_details bytes.Buffer

var exitrc = 0
var run_count = 0
var failed_count = 0
var unknown_count = 0

const Success = 0
const Error = 1
const Failure = 2

type ResultCode struct {
    code int
    what, how string
}

type LI struct {
    name string
    stepname string
    k ResultCode
    saved_failures bytes.Buffer
}

func (j *LI) log_failure(oops string) {
    if j.saved_failures.Len() > 0 {
	j.saved_failures.WriteString(", ")
    }
    j.saved_failures.WriteString(oops)
}

func initialize_log() {
    lf, err := ioutil.TempFile("", "t4_")
    if err != nil {
	panic("can't open temp file")
    }
    logfile_fp = lf
    err = os.Remove(logfile_fp.Name())
    if (err != nil) {
	fmt.Fprintf(os.Stderr, "Failed to remove %v err=%v\n", logfile_fp.Name(), err)
    }
}

func (j *LI) logfile_has_data() bool {
    o, err := logfile_fp.Seek(0, os.SEEK_CUR)
    if (err != nil) {
	j.log_failure(fmt.Sprintf("Failed to seek %v err=%v\n", logfile_fp.Name(), err))
	return true;
    }
    return o != 0
}

func intercept_log(name string) *LI {
    var rs LI
    run_count ++
    initialize_once.Do(initialize_log)
    trace_log.Reset()
    fmt.Printf("%v:", name)
    logfile_fp.Seek(0,0)
    logfile_fp.Truncate(0)
    log.SetOutput(logfile_fp)
    rs.name = name
    return &rs
}

func (j *LI) SetResult (k int, how string) {
    if k > j.k.code {
	j.k.code = k
	j.k.what = j.stepname
	j.k.how = how
    }
}

func (j *LI) SaveandSetResult (k int, how string) {
    j.log_failure(how)
    j.SetResult(k, how)
}

func (j *LI) Stepname(sn string) {
    j.stepname = sn
    trace_log.WriteString(fmt.Sprintf("\t-- %v\n", sn))
}

func (j *LI) release_log() int {
    var buffer bytes.Buffer
    data := make([]byte, 4096)
    var out int
    var total int
    var ok error
    j.stepname = "release_log"
    log.SetOutput(os.Stderr)
    logfile_fp.Seek(0,0)
    for {
	out, ok = logfile_fp.Read(data)
	if ok != nil {
	    if (ok == io.EOF) {
		ok = nil
	    } else {
		j.SaveandSetResult(Failure, fmt.Sprintf("problem reading log err=%v", ok))
	    }
	    break
	} else {
	    if (out != 0) {
		j.SaveandSetResult(Error, fmt.Sprintf("unexpected log output from sdk"))
		buffer.WriteString(fmt.Sprintf("%s", data[0:out]))
		total += out
	    }
	}
    }
    if ok != nil {
	j.log_failure(fmt.Sprintf( "can't read from pipe: err=%v\n", ok))
	j.SaveandSetResult(Error, fmt.Sprintf("can't read from pipe: err=%v\n", ok))
    }
    if j.k.code == Success {
	fmt.Println(" ok")
    } else if (j.k.code == Error) {
	fmt.Printf(" %v: %v: FAILED\n", j.k.what, j.k.how)
	failed_count++
	if (exitrc < 1) {
	    exitrc = 1
	}
    } else {
	fmt.Printf(" %v: %v: ?\n", j.k.what, j.k.how)
	unknown_count++
	if (exitrc < 2) {
	    exitrc = 2
	}
    }
    if j.k.code != Success {
        saved_details.WriteString(fmt.Sprintf ("\n** %v\n\n", j.name))
	if j.saved_failures.Len() > 0 {
	    saved_details.WriteString(fmt.Sprintf ("%s\n",j.saved_failures.String()))
	}
	if trace_log.Len() > 0 {
	    saved_details.WriteString(fmt.Sprintf ("Trace Log:\n%s\n",trace_log.String()))
	}
	if total != 0 {
	    saved_details.WriteString(fmt.Sprintf ("Unexpected log output (errors logged by ncw/swift):\n%s", buffer.String()))
	}
    }
    return j.k.code
}

var my_saved_connect *swift.Connection

type foo_transport struct {
    current *http.Request
}
func (t *foo_transport) RoundTrip(req *http.Request) (*http.Response, error) {
    var txt []byte
    t.current = req
    txt, err := httputil.DumpRequestOut(req, true)
if !*Dflag {} else { fmt.Printf("req = %q\n", txt) }
    trace_log.WriteString(fmt.Sprintf ("req = %q\n", txt))
    r, e := http.DefaultTransport.RoundTrip(req)
if (r != nil) {
	txt, err = httputil.DumpResponse(r, true)
	if *Dflag {
		fmt.Printf("response,e = %q,%v\n", txt,e)
	}
	trace_log.WriteString(fmt.Sprintf ("response,e = %q,%v\n", txt, e))
} else {
	if *Dflag {
		fmt.Printf("response,e = %v,%v\n", r,e)
	}
}
    _ = err
    return r, e
}

func (j *LI) get_swift_connection() *swift.Connection {
    if my_saved_connect != nil { return my_saved_connect }
    c := swift.Connection{
	UserName: st_user,
	ApiKey:   st_key,
	AuthUrl:   st_auth,
        AuthVersion: *st_auth_version,
    }
//    c.Transport = &http.Transport{
//	Proxy: http.ProxyFromEnvironment,
//		MaxIdleConnsPerHost: 2048,
 //   }
    c.Transport = &foo_transport{}
    j.Stepname("authenticate")
    err := c.Authenticate()
    if err != nil {
	j.SaveandSetResult(Failure, fmt.Sprintf("%v", err))
	return nil
    }
    if j.logfile_has_data() { return nil }
    my_saved_connect = &c
    return my_saved_connect
}

func (j *LI) make_pan_egg(c *swift.Connection) bool {
    saved_c := c
    cleanup_pan_egg.Do(func() {add_cleanup(func(){
//	fmt.Printf("About to cleanup pan and egg\n")
	err := saved_c.ObjectDelete("pan", "egg")
//	fmt.Printf("deleting object: err=%v\n", err)
	err = saved_c.ContainerDelete("pan")
//	fmt.Printf("deleting container: err=%v\n", err)
	_ = err
//	fmt.Printf("done cleanup pan!\n")
    })})
    j.Stepname("ContainerCreate pan")
    err := c.ContainerCreate("pan", nil)
    if err != nil {
	j.SaveandSetResult(Failure, fmt.Sprintf("err=%v", err))
	return false
    }
    if j.logfile_has_data() { return false }
    j.Stepname("ObjectCreate pan/egg")
    cf, err := c.ObjectCreate("pan", "egg", true, "", "", nil)
    if err != nil {
	j.SaveandSetResult(Failure, fmt.Sprintf("c err=%v", err))
	return false
    }
    if j.logfile_has_data() { return false }
    fmt.Fprintf(cf, "Tomato leaves may not be as dangerous as claimed")
    err = cf.Close()
    if (err != nil) {
	j.SaveandSetResult(Failure, fmt.Sprintf("w err=%v", err))
    }
    if j.logfile_has_data() { return false }
    return j.k.code == Success
}

func test_container_list() int {
    j := intercept_log("test_container_list")
    // Create a connection
//    if (*Dflag) {fmt.Printf("I am debugging; user=%v\n", st_user)
///	} else { fmt.Printf("I am not debugging; user=%v\n", st_user) }
//   fmt.Printf ("key=%v auth=%v auth_version=%v\n", st_key, st_auth, *st_auth_version)
    c := j.get_swift_connection()
    if c == nil {
	return j.release_log()
    }
    // List all the containers
    j.Stepname("ContainerNames")
    containers, err := c.ContainerNames(nil)
//    fmt.Println(containers)
    // etc...
    _ = containers
    if (err != nil) {
	j.SaveandSetResult(Error, fmt.Sprintf("err=%v", err))
    }
    return j.release_log()
}

func test_object_list_no_unsolicited_newline() int {
    j := intercept_log("test_object_list_no_unsolicited_newline")
    // Create a connection
    c := j.get_swift_connection()
    if c == nil {
	return j.release_log()
    }
    if !j.make_pan_egg(c) {
	return j.release_log()
    }
    // List all the objects
    j.Stepname("ObjectNames pan")
    objects, err := c.ObjectNames("pan", nil)
//    fmt.Println(objects)
    // etc...
    _ = objects
    if (err != nil) {
	j.SaveandSetResult(Error, fmt.Sprintf("err=%v", err))
    }
    return j.release_log()
}

func test_error_behaves_right() int {
    j := intercept_log("test_error_behaves_right")
    // Create a connection
    c := j.get_swift_connection()
    if c == nil {
	return j.release_log()
    }
    // Try to read an object
    j.Stepname("ObjectGetBytes no_such_container/no_such_object")
    contents, err := c.ObjectGetBytes("no_such_container", "no_such_object")
    if (err != swift.ObjectNotFound) {
        log.Printf("did not get object not found; got err=%v\n", err)
	j.SaveandSetResult(Failure, fmt.Sprintf("did not get object not found: err=%v", err))
    }
    _ = contents
//    // use connection for something else
//    j.Stepname("ContainerNames")
//    containers, err := c.ContainerNames(nil)
//    _ = containers
    return j.release_log()
}

func parse_opts() {
    errors := false
    flag.StringVar(&st_auth, "A", os.ExpandEnv("$ST_AUTH"), "swift auth")
    flag.StringVar(&st_user, "U", os.ExpandEnv("$ST_USER"), "swift user")
    flag.StringVar(&st_key, "K", os.ExpandEnv("$ST_KEY"), "swift key")
    i, ok := strconv.Atoi(os.ExpandEnv("$ST_AUTH_VERSION"))
    if ok != nil {
	i = 0
    }
    st_auth_version = flag.Int("V", i, "auth version")
    Dflag = flag.Bool("D", false, "debug flag (print everything)")
    vflag = flag.Bool("v", false, "verbose (print details on errors)")
    flag.Parse()
    if (st_auth == "") {
	fmt.Fprintf(os.Stderr, "Must specify auth, either cmdline or $ST_AUTH\n")
	errors = true
    }
    if (st_user == "") {
	fmt.Fprintf(os.Stderr, "Must specify user, either cmdline or $ST_USER\n")
	errors = true
    }
    if (st_key == "") {
	fmt.Fprintf(os.Stderr, "Must specify key, either cmdline or $ST_KEY\n")
	errors = true
    }
    if (errors) {
	flag.Usage()
	os.Exit(1)
    }
}
func print_test_summary() {
    var buffer bytes.Buffer
    run_plural := "";if run_count != 1 { run_plural = "s" }
    failed_plural := "";if failed_count != 1 { failed_plural = "s" }
    buffer.WriteString(fmt.Sprintf("%d test%s run; %d test%s failed",
	run_count, run_plural,
	failed_count, failed_plural))
    if (unknown_count > 0) {
	unknown_plural := "";if unknown_count != 1 { unknown_plural = "s" }
	buffer.WriteString(fmt.Sprintf(", %d test%s could not be completed",
	    unknown_count, unknown_plural))
    }
    fmt.Printf ("%s\n", buffer.String())
}

func print_saved_details() {
    if (saved_details.Len() == 0) {
	return
    }
    fmt.Printf ("Details\n%s", saved_details.String())
}


type func_list []func()
func(h func_list) Len() int	{ return len(h) }
func (h func_list) Swap(i,j int) { h[i], h[j] = h[j], h[i] }
func (h *func_list) Push(x interface{}) {
    *h = append(*h, x.(func()))
}
func (h *func_list) Pop() interface{} {
    old := *h
    n := len(old)
    x := old[n-1]
    *h = old[0:n-1]
    return x
}

var cleanup_list func_list

func add_cleanup(x func()) {
    cleanup_list.Push(x)
}

func test_cleanup() {
    for _, tf := range cleanup_list {
	tf()
    }
}

type T func()int
func main() {
    parse_opts()
    for _, tf := range[] T{
	test_container_list,
	test_object_list_no_unsolicited_newline,
	test_error_behaves_right,} {
	tf()
    }
    test_cleanup()
    print_test_summary()
    if *vflag {
	print_saved_details()
        fmt.Printf ("\n")
	print_test_summary()
    } else {
	if (saved_details.Len() != 0) {
	    fmt.Printf ("Rerun this command with -v to see more detail\n")
	}
    }
    os.Exit(exitrc)
}
