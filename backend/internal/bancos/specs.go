package bancos

// Especificaciones declarativas por banco. Columnas 0-based. Ver docs/GPVDP_Formatos_Bancos_v1.0.md.

var promerica = &bankSpec{
	banco:    BancoPromerica,
	sigToken: "promerica",
	colFecha: 0, colDoc: 1, colDesc: 2, colDebito: 3, colCredito: 4,
	fecha: fechaLayout("01-02-06"), // MM-DD-YY
	isHeader: func(c []string) bool {
		return cellHas(c, 0, "fecha") && cellHas(c, 1, "documento") && cellHas(c, 3, "debito")
	},
}

var bn = &bankSpec{
	banco:    BancoBN,
	colFecha: 1, colDoc: 2, colDesc: 5, colDebito: 3, colCredito: 4,
	fecha: fechaLayout("01-02-06"), // MM-DD-YY
	isHeader: func(c []string) bool {
		return cellHas(c, 0, "oficina") && cellHas(c, 1, "fechamovimiento") && cellHas(c, 3, "debito")
	},
}

var bac = &bankSpec{
	banco:    BancoBAC,
	sigToken: "detalle de movimientos",
	colFecha: 0, colDoc: 1, colDesc: 4, colDebito: 7, colCredito: 8,
	fecha: fechaLayout("02/01/2006"), // DD/MM/YYYY
	isHeader: func(c []string) bool {
		return cellHas(c, 0, "fecha") && cellHas(c, 1, "referencia") && cellHas(c, 7, "debito")
	},
}

var bcr = &bankSpec{
	banco:    BancoBCR,
	sigToken: "movimientos de cuenta",
	colFecha: 0, colDoc: 3, colDesc: 4, colDebito: 6, colCredito: 7,
	fecha: fechaLayout("01-02-06"), // MM-DD-YY
	isHeader: func(c []string) bool {
		return cellHas(c, 0, "fecha contable") && cellHas(c, 6, "debito")
	},
}

var bp = &bankSpec{
	banco:    BancoBP,
	sigToken: "banco popular",
	colFecha: 0, colDoc: 2, colDesc: 1, colDebito: 3, colCredito: 4,
	fecha: fechaBP, // "01 JUN 2026 03:23"
	isHeader: func(c []string) bool {
		return cellHas(c, 0, "fecha") && cellHas(c, 1, "descripcion") && cellHas(c, 3, "debito")
	},
}

var davivienda = &bankSpec{
	banco:    BancoDavivienda,
	colFecha: 0, colDoc: 2, colDesc: 1, colDebito: 3, colCredito: 4,
	fecha: fechaLayout("02/01/2006"), // DD/MM/YYYY
	isHeader: func(c []string) bool {
		// Encabezado único: "Débitos (DR)" en col D.
		return cellHas(c, 0, "fecha") && cellHas(c, 3, "debitos") && cellHas(c, 3, "dr")
	},
}
