package crawler

import "net/url"

type RetriableError struct{ Err error }

func (e RetriableError) Error() string {
	return e.Err.Error()
}

type StoreError struct{ err error }

func (e StoreError) Error() string {
	return e.err.Error()
}

func storeErr(err error) error {
	if err != nil {
		return StoreError{err}
	}
	return nil
}

type storeWrapper struct{ store Store }

func (w storeWrapper) Exist(u *url.URL) (bool, error) {
	v, err := w.store.Exist(u)
	return v, storeErr(err)
}
func (w storeWrapper) Get(u *url.URL) (*URL, error) {
	v, err := w.store.Get(u)
	return v, storeErr(err)
}
func (w storeWrapper) GetDepth(u *url.URL) (int, error) {
	v, err := w.store.GetDepth(u)
	return v, storeErr(err)
}
func (w storeWrapper) PutNX(u *URL) (bool, error) {
	v, err := w.store.PutNX(u)
	return v, storeErr(err)
}
func (w storeWrapper) Update(u *URL) error {
	err := w.store.Update(u)
	return storeErr(err)
}
func (w storeWrapper) UpdateStatus(u *url.URL, status int) error {
	err := w.store.UpdateStatus(u, status)
	return storeErr(err)
}
func (w storeWrapper) IncVisitCount() error {
	err := w.store.IncVisitCount()
	return storeErr(err)
}
func (w storeWrapper) IncErrorCount() error {
	err := w.store.IncErrorCount()
	return storeErr(err)
}
func (w storeWrapper) IsFinished() (bool, error) {
	v, err := w.store.IsFinished()
	return v, storeErr(err)
}
