package crawler

import "net/url"

type RetryableError struct{ Err error }
type FatalError struct{ Err error }

func (e RetryableError) Error() string {
	return e.Err.Error()
}
func (e FatalError) Error() string {
	return e.Err.Error()
}

func storeErr(err error) error {
	if err != nil {
		return FatalError{err}
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
func (w storeWrapper) GetFunc(u *url.URL, f func(*URL)) error {
	err := w.store.GetFunc(u, f)
	return storeErr(err)
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
func (w storeWrapper) UpdateFunc(u *url.URL, f func(*URL)) error {
	err := w.store.UpdateFunc(u, f)
	return storeErr(err)
}
func (w storeWrapper) Complete(u *url.URL) error {
	err := w.store.Complete(u)
	return storeErr(err)
}
func (w storeWrapper) IncVisitCount() error {
	err := w.store.IncVisitCount()
	return storeErr(err)
}
func (w storeWrapper) IsFinished() (bool, error) {
	v, err := w.store.IsFinished()
	return v, storeErr(err)
}
