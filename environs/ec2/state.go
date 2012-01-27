package ec2

const stateFile = "provider-state"

type bootstrapState struct {
	ZookeeperInstances []string	`yaml:"zookeeper-instances"`
}

func (e *environ) saveState(state bootstrapState) error {
}

func (e *environ) loadState() (bootstrapState, error) {
}

func (e *environ) deleteState() error {
	// TODO delete the bucket contents and the bucket itself.
	err := e.bucket().Del(stateFile)
	// If we can't delete the object because the bucket doesn't
	// exist, then we don't care.
	if err, _ := err.(*s3.Error); err != nil && err.StatusCode == 404 {
		return nil
	}
	return fmt.Errorf("cannot delete provider state: %v", err)
}

// makeBucket makes the environent's control bucket, the
// place where bootstrap information and deployed charms
// are stored. To avoid two round trips on every PUT operation,
// we do this only once for each environ.
func (e *environ) makeBucket() error {
	e.checkBucket.Do(func() {
		// try to make the bucket - PutBucket will succeed if the
		// bucket already exists.
		e.checkBucketError = e.bucket().PutBucket(s3.Private)
	})
	return e.checkBucketError
}

func (e *environ) PutFile(file string, r io.Reader, length int64) error {
	if err := e.makeBucket(); err != nil {
		return fmt.Errorf("cannot make S3 control bucket: %v", err)
	}
	err := e.bucket().PutReader(file, r, length, "binary/octet-stream", s3.Private)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control bucket: %v", file, err)
	}
	return nil
}

func (e *environ) GetFile(file string) (io.ReadCloser, error) {
	return e.bucket().GetReader(file)
}

func (e *environ) RemoveFile(file string) error {
	return e.bucket().Del(file)
}

func (e *environ) bucket() *s3.Bucket {
	return e.s3.Bucket(e.config.bucket)
}
