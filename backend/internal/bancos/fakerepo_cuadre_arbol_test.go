package bancos

import "context"

func (f *fakeRepo) CuadreArbol(context.Context, string, string) ([]CuadreArbolRow, error) {
	return nil, nil
}
