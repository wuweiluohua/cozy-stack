package instance

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/spf13/afero"
)

const globalDBPrefix = "global/"
const instanceType = "instances"

const globalFsURL = "file://localhost/tmp/cozy2"

// An Instance has the informations relatives to the logical cozy instance,
// like the domain, the locale or the access to the databases and files storage
// It is a couchdb.Doc to be persisted in couchdb.
type Instance struct {
	DocID      string `json:"_id,omitempty"`  // couchdb _id
	DocRev     string `json:"_rev,omitempty"` // couchdb _rev
	Domain     string `json:"domain"`         // The main DNS domain, like example.cozycloud.cc
	StorageURL string `json:"storage"`        // Where the binaries are persisted
	storage    afero.Fs
}

// DocType implements couchdb.Doc
func (i *Instance) DocType() string { return instanceType }

// ID implements couchdb.Doc
func (i *Instance) ID() string { return i.DocID }

// SetID implements couchdb.Doc
func (i *Instance) SetID(v string) { i.DocID = v }

// Rev implements couchdb.Doc
func (i *Instance) Rev() string { return i.DocRev }

// SetRev implements couchdb.Doc
func (i *Instance) SetRev(v string) { i.DocRev = v }

// ensure Instance implements couchdb.Doc
var _ couchdb.Doc = (*Instance)(nil)

// CreateInCouchdb create the instance doc in the global database
func (i *Instance) createInCouchdb() (err error) {
	err = couchdb.CreateDoc(globalDBPrefix, i)
	if err != nil {
		return err
	}
	byDomain := mango.IndexOnFields("domain")
	return couchdb.DefineIndex(globalDBPrefix, instanceType, byDomain)
}

// createRootFolder creates the root folder for this instance
func (i *Instance) createRootFolder() error {
	vfsC, err := i.GetVFSContext()
	if err != nil {
		return err
	}

	rootURL, err := url.Parse(globalFsURL)
	if err != nil {
		return err
	}

	rootFs, err := createFs(rootURL)
	if err != nil {
		return err
	}

	if err = rootFs.Mkdir(i.Domain, 0755); err != nil {
		return err
	}

	if err = vfs.CreateRootDirDoc(vfsC); err != nil {
		rootFs.Remove(i.Domain)
		return err
	}

	return nil
}

// createFSIndexes creates the index needed by VFS
func (i *Instance) createFSIndexes() (err error) {
	prefix := i.GetDatabasePrefix()
	byParent := mango.IndexOnFields("folder_id", "name", "type")
	byPath := mango.IndexOnFields("path")
	err = couchdb.DefineIndex(prefix, vfs.FsDocType, byParent)
	if err != nil {
		return err
	}
	err = couchdb.DefineIndex(prefix, vfs.FsDocType, byPath)
	return err
}

// Create build an instance and .Create it
func Create(domain string, locale string, apps []string) (*Instance, error) {
	if strings.ContainsAny(domain, vfs.ForbiddenFilenameChars) || domain == ".." || domain == "." {
		return nil, fmt.Errorf("Domain is malformed")
	}

	storageURL, err := url.Parse(globalFsURL)
	if err != nil {
		return nil, err
	}

	storageURL.Path = path.Join(storageURL.Path, domain)

	i := &Instance{
		Domain:     domain,
		StorageURL: storageURL.String(),
	}

	err = i.Create()
	if err != nil {
		return nil, err
	}

	return i, nil
}

// Create performs the necessary setups for this instance to be usable
func (i *Instance) Create() error {
	if err := i.createInCouchdb(); err != nil {
		return err
	}

	if err := i.createRootFolder(); err != nil {
		return err
	}

	if err := i.createFSIndexes(); err != nil {
		return err
	}

	// TODO atomicity with defer
	// TODO figure out what to do with locale
	// TODO install apps

	return nil
}

// Get retrieves the instance for a request by its host.
func Get(domain string) (*Instance, error) {
	// TODO this is not fail-safe, to be modified before production
	if domain == "" || strings.Contains(domain, "127.0.0.1") || strings.Contains(domain, "localhost") {
		domain = "dev"
	}

	var instances []*Instance
	req := &couchdb.FindRequest{
		Selector: mango.Equal("domain", domain),
		Limit:    1,
	}
	err := couchdb.FindDocs(globalDBPrefix, instanceType, req, &instances)
	if couchdb.IsNoDatabaseError(err) {
		return nil, fmt.Errorf("No instance for domain %v, use 'cozy-stack instances add'", domain)
	}
	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("No instance for domain %v, use 'cozy-stack instances add'", domain)
	}

	return instances[0], nil
}

// GetStorageProvider returns the afero storage provider where the binaries for
// the current instance are persisted
func (i *Instance) GetStorageProvider() (afero.Fs, error) {
	if i.storage != nil {
		return i.storage, nil
	}
	storageURL, err := url.Parse(i.StorageURL)
	if err != nil {
		return nil, err
	}
	i.storage, err = createFs(storageURL)
	return i.storage, err
}

// GetDatabasePrefix returns the prefix to use in database naming for the
// current instance
func (i *Instance) GetDatabasePrefix() string {
	return i.Domain + "/"
}

// GetVFSContext returns a vfs.Context for this Instance
func (i *Instance) GetVFSContext() (c *vfs.Context, err error) {
	dbprefix := i.GetDatabasePrefix()
	fs, err := i.GetStorageProvider()
	if err != nil {
		return nil, err
	}
	return vfs.NewContext(fs, dbprefix), nil
}

func createFs(u *url.URL) (fs afero.Fs, err error) {
	switch u.Scheme {
	case "file":
		fs = afero.NewBasePathFs(afero.NewOsFs(), u.Path)
	case "mem":
		fs = afero.NewMemMapFs()
	default:
		err = fmt.Errorf("Unknown storage provider: %v", u.Scheme)
	}
	return
}
