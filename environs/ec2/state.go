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
	err := e.controlBucket().Del(stateFile)
	// If we can't delete the object because the bucket doesn't
	// exist, then we don't care.
	if err, _ := delErr.(*s3.Error); err != nil && err.StatusCode == 404 {
		return nil
	}
	return fmt.Errorf("cannot delete provider state: %v", err)
}

func (e *environ) makeControlBucket() error {
	e.checkBucket.Do(func() {
		b := e.controlBucket()
		// As bucket LIST isn't implemented for the s3test server yet,
		// we try to get an object from the control bucket
		// and determine whether the bucket exists using the resulting
		// error message.
		r, testErr := b.GetReader("testing123")
		if testErr == nil {
			r.Close()
			return
		}
		if testErr, _ := testErr.(*s3.Error); testErr == nil || testErr.Code != "NoSuchBucket" {
			return
		}
		// The bucket doesn't exist, so try to make it.
		e.checkBucketError = b.PutBucket(s3.Private)
	})
	return e.checkBucketError
}

func (e *environ) PutFile(file string, r io.Reader, length int64) error {
	if err := e.makeControlBucket(); err != nil {
		return fmt.Errorf("cannot make S3 control bucket: %v", err)
	}
	err := e.controlBucket().PutReader(file, r, length, "binary/octet-stream", s3.Private)
	if err != nil {
		return fmt.Errorf("cannot write file %q to control bucket: %v", file, err)
	}
	return nil
}

func (e *environ) GetFile(file string) (io.ReadCloser, error) {
	return e.controlBucket().GetReader(file)
}

func (e *environ) RemoveFile(file string) error {
	return e.controlBucket().Del(file)
}

func (e *environ) controlBucket() *s3.Bucket {
	return e.s3.Bucket(e.config.controlBucket)
}
