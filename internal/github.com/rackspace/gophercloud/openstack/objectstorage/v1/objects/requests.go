package objects

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud"
	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/openstack/objectstorage/v1/accounts"
	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/pagination"
)

// ListOptsBuilder allows extensions to add additional parameters to the List
// request.
type ListOptsBuilder interface {
	ToObjectListParams() (bool, string, error)
}

// ListOpts is a structure that holds parameters for listing objects.
type ListOpts struct {
	// Full is a true/false value that represents the amount of object information
	// returned. If Full is set to true, then the content-type, number of bytes, hash
	// date last modified, and name are returned. If set to false or not set, then
	// only the object names are returned.
	Full      bool
	Limit     int    `q:"limit"`
	Marker    string `q:"marker"`
	EndMarker string `q:"end_marker"`
	Format    string `q:"format"`
	Prefix    string `q:"prefix"`
	Delimiter string `q:"delimiter"`
	Path      string `q:"path"`
}

// ToObjectListParams formats a ListOpts into a query string and boolean
// representing whether to list complete information for each object.
func (opts ListOpts) ToObjectListParams() (bool, string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	if err != nil {
		return false, "", err
	}
	return opts.Full, q.String(), nil
}

// List is a function that retrieves all objects in a container. It also returns the details
// for the container. To extract only the object information or names, pass the ListResult
// response to the ExtractInfo or ExtractNames function, respectively.
func List(c *gophercloud.ServiceClient, containerName string, opts ListOptsBuilder) pagination.Pager {
	headers := map[string]string{"Accept": "text/plain", "Content-Type": "text/plain"}

	url := listURL(c, containerName)
	if opts != nil {
		full, query, err := opts.ToObjectListParams()
		if err != nil {
			return pagination.Pager{Err: err}
		}
		url += query

		if full {
			headers = map[string]string{"Accept": "application/json", "Content-Type": "application/json"}
		}
	}

	createPage := func(r pagination.PageResult) pagination.Page {
		p := ObjectPage{pagination.MarkerPageBase{PageResult: r}}
		p.MarkerPageBase.Owner = p
		return p
	}

	pager := pagination.NewPager(c, url, createPage)
	pager.Headers = headers
	return pager
}

// DownloadOptsBuilder allows extensions to add additional parameters to the
// Download request.
type DownloadOptsBuilder interface {
	ToObjectDownloadParams() (map[string]string, string, error)
}

// DownloadOpts is a structure that holds parameters for downloading an object.
type DownloadOpts struct {
	IfMatch           string    `h:"If-Match"`
	IfModifiedSince   time.Time `h:"If-Modified-Since"`
	IfNoneMatch       string    `h:"If-None-Match"`
	IfUnmodifiedSince time.Time `h:"If-Unmodified-Since"`
	Range             string    `h:"Range"`
	Expires           string    `q:"expires"`
	MultipartManifest string    `q:"multipart-manifest"`
	Signature         string    `q:"signature"`
}

// ToObjectDownloadParams formats a DownloadOpts into a query string and map of
// headers.
func (opts DownloadOpts) ToObjectDownloadParams() (map[string]string, string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	if err != nil {
		return nil, "", err
	}
	h, err := gophercloud.BuildHeaders(opts)
	if err != nil {
		return nil, q.String(), err
	}
	return h, q.String(), nil
}

// Download is a function that retrieves the content and metadata for an object.
// To extract just the content, pass the DownloadResult response to the
// ExtractContent function.
func Download(c *gophercloud.ServiceClient, containerName, objectName string, opts DownloadOptsBuilder) DownloadResult {
	var res DownloadResult

	url := downloadURL(c, containerName, objectName)
	h := c.AuthenticatedHeaders()

	if opts != nil {
		headers, query, err := opts.ToObjectDownloadParams()
		if err != nil {
			res.Err = err
			return res
		}

		for k, v := range headers {
			h[k] = v
		}

		url += query
	}

	resp, err := c.Request("GET", url, gophercloud.RequestOpts{
		MoreHeaders: h,
		OkCodes:     []int{200, 304},
	})
	if resp != nil {
		res.Header = resp.Header
		res.Body = resp.Body
	}
	res.Err = err

	return res
}

// CreateOptsBuilder allows extensions to add additional parameters to the
// Create request.
type CreateOptsBuilder interface {
	ToObjectCreateParams() (map[string]string, string, error)
}

// CreateOpts is a structure that holds parameters for creating an object.
type CreateOpts struct {
	Metadata           map[string]string
	ContentDisposition string `h:"Content-Disposition"`
	ContentEncoding    string `h:"Content-Encoding"`
	ContentLength      int64  `h:"Content-Length"`
	ContentType        string `h:"Content-Type"`
	CopyFrom           string `h:"X-Copy-From"`
	DeleteAfter        int    `h:"X-Delete-After"`
	DeleteAt           int    `h:"X-Delete-At"`
	DetectContentType  string `h:"X-Detect-Content-Type"`
	ETag               string `h:"ETag"`
	IfNoneMatch        string `h:"If-None-Match"`
	ObjectManifest     string `h:"X-Object-Manifest"`
	TransferEncoding   string `h:"Transfer-Encoding"`
	Expires            string `q:"expires"`
	MultipartManifest  string `q:"multipart-manifest"`
	Signature          string `q:"signature"`
}

// ToObjectCreateParams formats a CreateOpts into a query string and map of
// headers.
func (opts CreateOpts) ToObjectCreateParams() (map[string]string, string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	if err != nil {
		return nil, "", err
	}
	h, err := gophercloud.BuildHeaders(opts)
	if err != nil {
		return nil, q.String(), err
	}

	for k, v := range opts.Metadata {
		h["X-Object-Meta-"+k] = v
	}

	return h, q.String(), nil
}

// Create is a function that creates a new object or replaces an existing object. If the returned response's ETag
// header fails to match the local checksum, the failed request will automatically be retried up to a maximum of 3 times.
func Create(c *gophercloud.ServiceClient, containerName, objectName string, content io.ReadSeeker, opts CreateOptsBuilder) CreateResult {
	var res CreateResult

	url := createURL(c, containerName, objectName)
	h := make(map[string]string)

	if opts != nil {
		headers, query, err := opts.ToObjectCreateParams()
		if err != nil {
			res.Err = err
			return res
		}

		for k, v := range headers {
			h[k] = v
		}

		url += query
	}

	hash := md5.New()
	bufioReader := bufio.NewReader(io.TeeReader(content, hash))
	io.Copy(ioutil.Discard, bufioReader)
	localChecksum := hash.Sum(nil)

	h["ETag"] = fmt.Sprintf("%x", localChecksum)

	_, err := content.Seek(0, 0)
	if err != nil {
		res.Err = err
		return res
	}

	ropts := gophercloud.RequestOpts{
		RawBody:     content,
		MoreHeaders: h,
	}

	resp, err := c.Request("PUT", url, ropts)
	if err != nil {
		res.Err = err
		return res
	}
	if resp != nil {
		res.Header = resp.Header
		if resp.Header.Get("ETag") == fmt.Sprintf("%x", localChecksum) {
			res.Err = err
			return res
		}
		res.Err = fmt.Errorf("Local checksum does not match API ETag header")
	}

	return res
}

// CopyOptsBuilder allows extensions to add additional parameters to the
// Copy request.
type CopyOptsBuilder interface {
	ToObjectCopyMap() (map[string]string, error)
}

// CopyOpts is a structure that holds parameters for copying one object to
// another.
type CopyOpts struct {
	Metadata           map[string]string
	ContentDisposition string `h:"Content-Disposition"`
	ContentEncoding    string `h:"Content-Encoding"`
	ContentType        string `h:"Content-Type"`
	Destination        string `h:"Destination,required"`
}

// ToObjectCopyMap formats a CopyOpts into a map of headers.
func (opts CopyOpts) ToObjectCopyMap() (map[string]string, error) {
	if opts.Destination == "" {
		return nil, fmt.Errorf("Required CopyOpts field 'Destination' not set.")
	}
	h, err := gophercloud.BuildHeaders(opts)
	if err != nil {
		return nil, err
	}
	for k, v := range opts.Metadata {
		h["X-Object-Meta-"+k] = v
	}
	return h, nil
}

// Copy is a function that copies one object to another.
func Copy(c *gophercloud.ServiceClient, containerName, objectName string, opts CopyOptsBuilder) CopyResult {
	var res CopyResult
	h := c.AuthenticatedHeaders()

	headers, err := opts.ToObjectCopyMap()
	if err != nil {
		res.Err = err
		return res
	}

	for k, v := range headers {
		h[k] = v
	}

	url := copyURL(c, containerName, objectName)
	resp, err := c.Request("COPY", url, gophercloud.RequestOpts{
		MoreHeaders: h,
		OkCodes:     []int{201},
	})
	if resp != nil {
		res.Header = resp.Header
	}
	res.Err = err
	return res
}

// DeleteOptsBuilder allows extensions to add additional parameters to the
// Delete request.
type DeleteOptsBuilder interface {
	ToObjectDeleteQuery() (string, error)
}

// DeleteOpts is a structure that holds parameters for deleting an object.
type DeleteOpts struct {
	MultipartManifest string `q:"multipart-manifest"`
}

// ToObjectDeleteQuery formats a DeleteOpts into a query string.
func (opts DeleteOpts) ToObjectDeleteQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	if err != nil {
		return "", err
	}
	return q.String(), nil
}

// Delete is a function that deletes an object.
func Delete(c *gophercloud.ServiceClient, containerName, objectName string, opts DeleteOptsBuilder) DeleteResult {
	var res DeleteResult
	url := deleteURL(c, containerName, objectName)

	if opts != nil {
		query, err := opts.ToObjectDeleteQuery()
		if err != nil {
			res.Err = err
			return res
		}
		url += query
	}

	resp, err := c.Delete(url, nil)
	if resp != nil {
		res.Header = resp.Header
	}
	res.Err = err
	return res
}

// GetOptsBuilder allows extensions to add additional parameters to the
// Get request.
type GetOptsBuilder interface {
	ToObjectGetQuery() (string, error)
}

// GetOpts is a structure that holds parameters for getting an object's metadata.
type GetOpts struct {
	Expires   string `q:"expires"`
	Signature string `q:"signature"`
}

// ToObjectGetQuery formats a GetOpts into a query string.
func (opts GetOpts) ToObjectGetQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	if err != nil {
		return "", err
	}
	return q.String(), nil
}

// Get is a function that retrieves the metadata of an object. To extract just the custom
// metadata, pass the GetResult response to the ExtractMetadata function.
func Get(c *gophercloud.ServiceClient, containerName, objectName string, opts GetOptsBuilder) GetResult {
	var res GetResult
	url := getURL(c, containerName, objectName)

	if opts != nil {
		query, err := opts.ToObjectGetQuery()
		if err != nil {
			res.Err = err
			return res
		}
		url += query
	}

	resp, err := c.Request("HEAD", url, gophercloud.RequestOpts{
		OkCodes: []int{200, 204},
	})
	if resp != nil {
		res.Header = resp.Header
	}
	res.Err = err
	return res
}

// UpdateOptsBuilder allows extensions to add additional parameters to the
// Update request.
type UpdateOptsBuilder interface {
	ToObjectUpdateMap() (map[string]string, error)
}

// UpdateOpts is a structure that holds parameters for updating, creating, or deleting an
// object's metadata.
type UpdateOpts struct {
	Metadata           map[string]string
	ContentDisposition string `h:"Content-Disposition"`
	ContentEncoding    string `h:"Content-Encoding"`
	ContentType        string `h:"Content-Type"`
	DeleteAfter        int    `h:"X-Delete-After"`
	DeleteAt           int    `h:"X-Delete-At"`
	DetectContentType  bool   `h:"X-Detect-Content-Type"`
}

// ToObjectUpdateMap formats a UpdateOpts into a map of headers.
func (opts UpdateOpts) ToObjectUpdateMap() (map[string]string, error) {
	h, err := gophercloud.BuildHeaders(opts)
	if err != nil {
		return nil, err
	}
	for k, v := range opts.Metadata {
		h["X-Object-Meta-"+k] = v
	}
	return h, nil
}

// Update is a function that creates, updates, or deletes an object's metadata.
func Update(c *gophercloud.ServiceClient, containerName, objectName string, opts UpdateOptsBuilder) UpdateResult {
	var res UpdateResult
	h := c.AuthenticatedHeaders()

	if opts != nil {
		headers, err := opts.ToObjectUpdateMap()
		if err != nil {
			res.Err = err
			return res
		}

		for k, v := range headers {
			h[k] = v
		}
	}

	url := updateURL(c, containerName, objectName)
	resp, err := c.Request("POST", url, gophercloud.RequestOpts{
		MoreHeaders: h,
	})
	if resp != nil {
		res.Header = resp.Header
	}
	res.Err = err
	return res
}

// HTTPMethod represents an HTTP method string (e.g. "GET").
type HTTPMethod string

var (
	// GET represents an HTTP "GET" method.
	GET HTTPMethod = "GET"
	// POST represents an HTTP "POST" method.
	POST HTTPMethod = "POST"
)

// CreateTempURLOpts are options for creating a temporary URL for an object.
type CreateTempURLOpts struct {
	// (REQUIRED) Method is the HTTP method to allow for users of the temp URL. Valid values
	// are "GET" and "POST".
	Method HTTPMethod
	// (REQUIRED) TTL is the number of seconds the temp URL should be active.
	TTL int
	// (Optional) Split is the string on which to split the object URL. Since only
	// the object path is used in the hash, the object URL needs to be parsed. If
	// empty, the default OpenStack URL split point will be used ("/v1/").
	Split string
}

// CreateTempURL is a function for creating a temporary URL for an object. It
// allows users to have "GET" or "POST" access to a particular tenant's object
// for a limited amount of time.
func CreateTempURL(c *gophercloud.ServiceClient, containerName, objectName string, opts CreateTempURLOpts) (string, error) {
	if opts.Split == "" {
		opts.Split = "/v1/"
	}
	duration := time.Duration(opts.TTL) * time.Second
	expiry := time.Now().Add(duration).Unix()
	getHeader, err := accounts.Get(c, nil).Extract()
	if err != nil {
		return "", err
	}
	secretKey := []byte(getHeader.TempURLKey)
	url := getURL(c, containerName, objectName)
	splitPath := strings.Split(url, opts.Split)
	baseURL, objectPath := splitPath[0], splitPath[1]
	objectPath = opts.Split + objectPath
	body := fmt.Sprintf("%s\n%d\n%s", opts.Method, expiry, objectPath)
	hash := hmac.New(sha1.New, secretKey)
	hash.Write([]byte(body))
	hexsum := fmt.Sprintf("%x", hash.Sum(nil))
	return fmt.Sprintf("%s%s?temp_url_sig=%s&temp_url_expires=%d", baseURL, objectPath, hexsum, expiry), nil
}

// CreateLargeOptsBuilder allows extensions to add additional parameters to the
// CreateLarge request.
type CreateLargeOptsBuilder interface {
	ToObjectCreateLargeParams() (map[string]string, string, error)
	SizeOfPieces() (int64, error)
	LengthOfContent() (int64, error)
	NumConcurrent() (int, error)
	ChannelOfStatuses() (chan *TransferStatus, error)
}

// CreateLargeOpts is a structure that holds parameters for creating a large object.
type CreateLargeOpts struct {
	CreateOpts
	// [REQUIRED] The size of the pieces to break the large object into (in MB).
	SizePieces int64
	// [OPTIONAL] The number of concurrent goroutines. Default is runtime.NumCPU()
	Concurrency int
	// [OPTIONAL] The channel on which to send status updates
	StatusChannel chan *TransferStatus
}

// ToObjectCreateLargeParams formats a CreateLargeOpts into a query string and map of
// headers.
func (opts CreateLargeOpts) ToObjectCreateLargeParams() (map[string]string, string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	if err != nil {
		return nil, "", err
	}
	h, err := gophercloud.BuildHeaders(opts)
	if err != nil {
		return nil, q.String(), err
	}

	for k, v := range opts.Metadata {
		h["X-Object-Meta-"+k] = v
	}

	return h, q.String(), nil
}

// SizeOfPieces returns the size that each piece of the uploaded object should
// have (except possibly the last one).
func (opts CreateLargeOpts) SizeOfPieces() (int64, error) {
	if opts.SizePieces == 0 {
		return 0, errors.New("SizePieces must be provided.")
	}
	return opts.SizePieces * 1000000, nil
}

// LengthOfContent returns the total length of the content to upload.
func (opts CreateLargeOpts) LengthOfContent() (int64, error) {
	return opts.ContentLength, nil
}

// NumConcurrent returns the number of concurrent goroutines allowed.
func (opts CreateLargeOpts) NumConcurrent() (int, error) {
	if opts.Concurrency == 0 {
		opts.Concurrency = 1
	}
	return opts.Concurrency, nil
}

// ChannelOfStatuses returns the channel on which to send status updates on the
// progress of the upload.
func (opts CreateLargeOpts) ChannelOfStatuses() (chan *TransferStatus, error) {
	if opts.StatusChannel == nil {
		return nil, errors.New("StatusChannel must be provided.")
	}
	return opts.StatusChannel, nil
}

// StatusMsg is the type of status message being sent on the channel returned by
// ChannelOfStatuses.
type StatusMsg string

var (
	StatusStarted StatusMsg = "started"
	StatusUpdate  StatusMsg = "update"
	StatusSuccess StatusMsg = "success"
	StatusError   StatusMsg = "error"
)

const defaultMaxRetries = 5

type uploadJob struct {
	id          int
	retriesLeft int
}

// TransferStatus is the status of an HTTP transfer.
type TransferStatus struct {
	Name              string
	TotalSize         int
	IncrementUploaded int
	StartTime         time.Time
	MsgType           StatusMsg
	Err               error
}

// contentWrapperTransferProgress is a wrapper to track the progress of an
// HTTP transfer such as a file upload or download.
type contentWrapperTransferProgress struct {
	userContent       io.Reader
	name              string
	totalSize         int
	incrementUploaded int
	startTime         time.Time
	responseChannel   chan *TransferStatus
}

func (cwtp contentWrapperTransferProgress) Read(p []byte) (int, error) {
	n, err := cwtp.userContent.Read(p)
	cwtp.responseChannel <- &TransferStatus{
		Name:              cwtp.name,
		TotalSize:         cwtp.totalSize,
		IncrementUploaded: n,
		MsgType:           StatusUpdate,
		StartTime:         cwtp.startTime,
	}
	return n, err
}

// CreateLarge is a function that creates a new large object or replaces an existing object. If the returned response's ETag
// header fails to match the local checksum, the request will fail.
func CreateLarge(c *gophercloud.ServiceClient, containerName, objectName string, content io.Reader, opts CreateLargeOptsBuilder) CreateResult {
	var res CreateResult

	// Get the size of the pieces
	sizePieces, err := opts.SizeOfPieces()
	if err != nil {
		res.Err = err
		return res
	}

	// Get the length of the content
	contentLength, err := opts.LengthOfContent()
	if err != nil {
		res.Err = err
		return res
	}

	// Get the number of concurrent goroutines to launch
	numConcurrent, err := opts.NumConcurrent()
	if err != nil {
		res.Err = err
		return res
	}

	statusChannel, err := opts.ChannelOfStatuses()
	if err != nil {
		res.Err = err
		return res
	}

	// Get the request headers and query string
	headers, query, err := opts.ToObjectCreateLargeParams()
	if err != nil {
		res.Err = err
		return res
	}

	// If the content satisfies the `io.ReaderAt` interface, we can safely read
	// it concurrently.
	if readerAt, ok := content.(io.ReaderAt); ok && contentLength != 0 && numConcurrent != 1 {

		// Calculate the number of pieces to upload.
		numPieces := int(contentLength / sizePieces)
		if contentLength%sizePieces != 0 {
			numPieces++
		}

		if numConcurrent > numPieces {
			numConcurrent = numPieces
		}

		uploadJobs := make(chan uploadJob, numPieces)
		var wg sync.WaitGroup
		for i := 0; i < numConcurrent; i++ {
			wg.Add(1)
			go func() {
				for job := range uploadJobs {
					sectionReader := io.NewSectionReader(readerAt, int64(job.id)*sizePieces, sizePieces)

					thisObject := fmt.Sprintf("%s.%03d", objectName, job.id)
					url := createURL(c, containerName, thisObject)
					url += query

					hash := md5.New()

					cwtp := contentWrapperTransferProgress{
						name:            thisObject,
						totalSize:       int(sizePieces),
						startTime:       time.Now(),
						responseChannel: statusChannel,
					}
					cwtp.userContent = io.TeeReader(sectionReader, hash)

					if job.id == numPieces-1 {
						cwtp.totalSize = int(contentLength % sizePieces)
					}

					headers["Content-Length"] = string(cwtp.totalSize)
					ropts := gophercloud.RequestOpts{
						RawBody:     cwtp,
						MoreHeaders: headers,
					}

					transferStatus := &TransferStatus{
						Name:              thisObject,
						TotalSize:         cwtp.totalSize,
						IncrementUploaded: cwtp.incrementUploaded,
						StartTime:         time.Now(),
					}

					transferStatus.MsgType = StatusStarted
					statusChannel <- transferStatus

					resp, err := c.Request("PUT", url, ropts)

					if closeable, ok := cwtp.userContent.(io.ReadCloser); ok {
						closeable.Close()
					}

					if err != nil {
						transferStatus.MsgType = StatusError
						transferStatus.Err = err
						statusChannel <- transferStatus
						uploadJobs <- uploadJob{job.id, job.retriesLeft - 1}
						continue
					}

					if resp != nil {
						if resp.Header.Get("ETag") == fmt.Sprintf("%x", hash.Sum(nil)) {
							transferStatus.MsgType = StatusSuccess
							statusChannel <- transferStatus
							if len(uploadJobs) != 0 {
								continue
							}
							time.Sleep(time.Second * 1)
							break
						}
						transferStatus.MsgType = StatusError
						transferStatus.Err = fmt.Errorf(fmt.Sprintf("Local checksum does not match API ETag header for file: %s", thisObject))
						statusChannel <- transferStatus
						uploadJobs <- uploadJob{job.id, job.retriesLeft - 1}
					}
				}
				wg.Done()
			}()
		}

		for i := 0; i < numPieces; i++ {
			uploadJobs <- uploadJob{i, defaultMaxRetries}
		}

		wg.Wait()

	} else {
		for i := 0; ; i++ {
			thisObject := fmt.Sprintf("%s.%03d", objectName, i)
			url := createURL(c, containerName, fmt.Sprintf("%s", thisObject))
			url += query

			hash := md5.New()
			limitReader := io.LimitReader(content, sizePieces)

			cwtp := contentWrapperTransferProgress{
				name:            thisObject,
				totalSize:       int(sizePieces),
				startTime:       time.Now(),
				responseChannel: statusChannel,
			}

			transferStatus := &TransferStatus{
				Name:              thisObject,
				IncrementUploaded: cwtp.incrementUploaded,
				StartTime:         time.Now(),
			}

			buf := bytes.NewBuffer([]byte{})
			_, err := io.Copy(io.MultiWriter(hash, buf), limitReader)

			if buf.Len() == 0 {
				break
			}

			transferStatus.TotalSize = buf.Len()

			if err != nil {
				transferStatus.MsgType = StatusError
				transferStatus.Err = err
				statusChannel <- transferStatus
				break
			}
			cwtp.userContent = buf

			localChecksum := fmt.Sprintf("%x", hash.Sum(nil))
			headers["ETag"] = localChecksum

			ropts := gophercloud.RequestOpts{
				RawBody:     cwtp,
				MoreHeaders: headers,
			}

			transferStatus.MsgType = StatusStarted
			statusChannel <- transferStatus

			resp, err := c.Request("PUT", url, ropts)
			if err != nil {
				transferStatus.MsgType = StatusError
				transferStatus.Err = err
				statusChannel <- transferStatus
				break
			}

			if closeable, ok := cwtp.userContent.(io.ReadCloser); ok {
				closeable.Close()
			}

			if err != nil {
				transferStatus.MsgType = StatusError
				transferStatus.Err = err
				statusChannel <- transferStatus
				continue
			}

			if resp != nil {
				if resp.Header.Get("ETag") == localChecksum {
					transferStatus.MsgType = StatusSuccess
					statusChannel <- transferStatus
				} else {
					transferStatus.MsgType = StatusError
					transferStatus.Err = fmt.Errorf(fmt.Sprintf("Local checksum does not match API ETag header for file: %s", thisObject))
					statusChannel <- transferStatus
				}
			}

		}

		ropts := gophercloud.RequestOpts{
			MoreHeaders: map[string]string{
				"X-Object-Manifest": fmt.Sprintf("%s/%s", containerName, objectName),
			},
		}

		resp, err := c.Request("PUT", createURL(c, containerName, objectName), ropts)

		if err != nil {
			statusChannel <- &TransferStatus{Err: err}
		}
		if resp != nil {
			res.Header = resp.Header
		}
	}

	close(statusChannel)
	return res
}

// ErrCreateLarge represents the errors returned from a CreateLarge operation.
type ErrCreateLarge []error

func (e ErrCreateLarge) Error() string {
	s := ""
	for _, err := range e {
		s += err.Error() + "\n"
	}
	return s
}
