--
-- PostgreSQL database dump
--

\restrict XeXZotFf7DC7oWQGqvydDNY2euUWx8DsUWDlUgitEzeh9oepTCqsuUidpEnriFq

-- Dumped from database version 16.14 (Debian 16.14-1.pgdg13+1)
-- Dumped by pg_dump version 16.14 (Debian 16.14-1.pgdg13+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: btree_gist; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public;


--
-- Name: EXTENSION btree_gist; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION btree_gist IS 'support for indexing common datatypes in GiST';


--
-- Name: pg_trgm; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;


--
-- Name: EXTENSION pg_trgm; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pg_trgm IS 'text similarity measurement and index searching based on trigrams';


--
-- Name: auditoria_no_truncate(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.auditoria_no_truncate() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'auditoria_evento es append-only: TRUNCATE no permitido';
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: acta_conciliacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.acta_conciliacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    cuenta_bancaria_id uuid NOT NULL,
    anio integer NOT NULL,
    mes integer NOT NULL,
    saldo_banco numeric(16,2) NOT NULL,
    saldo_libros numeric(16,2) NOT NULL,
    ajuste_partidas numeric(16,2) NOT NULL,
    preparado_por uuid,
    preparado_en timestamp with time zone DEFAULT now() NOT NULL,
    firmado_por uuid,
    firmado_en timestamp with time zone
);


--
-- Name: anticipo_aplicacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.anticipo_aplicacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    anticipo_id uuid NOT NULL,
    factura_id uuid NOT NULL,
    monto_crc numeric(14,2) NOT NULL,
    aplicado_por uuid,
    aplicado_en timestamp with time zone DEFAULT now() NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    reversado_por uuid,
    reversado_en timestamp with time zone,
    CONSTRAINT anticipo_aplicacion_check CHECK ((anticipo_id <> factura_id)),
    CONSTRAINT anticipo_aplicacion_monto_crc_check CHECK ((monto_crc > (0)::numeric))
);


--
-- Name: arreglo_cuota_cxc; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arreglo_cuota_cxc (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    arreglo_id uuid NOT NULL,
    numero integer NOT NULL,
    vence_en date NOT NULL,
    monto numeric(18,2) NOT NULL,
    CONSTRAINT arreglo_cuota_cxc_monto_check CHECK ((monto > (0)::numeric)),
    CONSTRAINT arreglo_cuota_cxc_numero_check CHECK ((numero >= 0))
);


--
-- Name: arreglo_pago_cxc; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.arreglo_pago_cxc (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    contrato_id uuid NOT NULL,
    consecutivo bigint NOT NULL,
    saldo_al_pactar numeric(18,2) NOT NULL,
    vencido_al_pactar numeric(18,2) NOT NULL,
    cuotas_vencidas_al_pactar integer DEFAULT 0 NOT NULL,
    meses_mora_al_pactar numeric(6,1) DEFAULT 0 NOT NULL,
    monto_arreglo numeric(18,2) NOT NULL,
    plazo_cuotas integer NOT NULL,
    prima numeric(18,2) DEFAULT 0 NOT NULL,
    es_excepcion boolean DEFAULT false NOT NULL,
    autorizado_por uuid,
    autorizacion_motivo text DEFAULT ''::text NOT NULL,
    quebrado_en timestamp with time zone,
    quebrado_por uuid,
    quebranto_motivo text DEFAULT ''::text NOT NULL,
    anulado_en timestamp with time zone,
    anulado_por uuid,
    anulacion_motivo text DEFAULT ''::text NOT NULL,
    observaciones text DEFAULT ''::text NOT NULL,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT arreglo_pago_cxc_monto_arreglo_check CHECK ((monto_arreglo > (0)::numeric)),
    CONSTRAINT arreglo_pago_cxc_plazo_cuotas_check CHECK ((plazo_cuotas >= 1)),
    CONSTRAINT arreglo_pago_cxc_prima_check CHECK ((prima >= (0)::numeric))
);


--
-- Name: auditoria_evento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.auditoria_evento (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid,
    entidad text NOT NULL,
    entidad_id uuid,
    accion text NOT NULL,
    valor_anterior jsonb,
    valor_nuevo jsonb,
    usuario_id uuid,
    ts timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: banco; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.banco (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: bccr_sync_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bccr_sync_log (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    fecha date NOT NULL,
    indicador text NOT NULL,
    valor numeric(14,4),
    exito boolean NOT NULL,
    mensaje text,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: caja_chica_fondo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.caja_chica_fondo (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    custodio_id uuid,
    departamento_id uuid,
    proveedor_id uuid,
    monto_asignado numeric(14,2) NOT NULL,
    umbral_pct numeric(5,2) DEFAULT 30 NOT NULL,
    limite_vale numeric(14,2) DEFAULT 0 NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT caja_chica_fondo_limite_vale_check CHECK ((limite_vale >= (0)::numeric)),
    CONSTRAINT caja_chica_fondo_monto_asignado_check CHECK ((monto_asignado > (0)::numeric)),
    CONSTRAINT caja_chica_fondo_umbral_pct_check CHECK (((umbral_pct >= (0)::numeric) AND (umbral_pct <= (100)::numeric)))
);


--
-- Name: caja_chica_vale; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.caja_chica_vale (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    fondo_id uuid NOT NULL,
    fecha date DEFAULT CURRENT_DATE NOT NULL,
    detalle text NOT NULL,
    monto_crc numeric(14,2) NOT NULL,
    concepto_id uuid,
    clasificacion_id uuid,
    subclasificacion_id uuid,
    comprobante text DEFAULT 'RECIBO'::text NOT NULL,
    registrado_por uuid,
    reposicion_id uuid,
    anulado boolean DEFAULT false NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT caja_chica_vale_comprobante_check CHECK ((comprobante = ANY (ARRAY['FE'::text, 'RECIBO'::text]))),
    CONSTRAINT caja_chica_vale_monto_crc_check CHECK ((monto_crc > (0)::numeric))
);


--
-- Name: cargo_cxc; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cargo_cxc (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    contrato_id uuid NOT NULL,
    periodo text NOT NULL,
    vence_en date NOT NULL,
    monto numeric(14,2) NOT NULL,
    monto_aplicado numeric(14,2) DEFAULT 0 NOT NULL,
    estado text DEFAULT 'ABIERTO'::text NOT NULL,
    origen text DEFAULT 'GENERADO'::text NOT NULL,
    clave_hacienda text,
    nota text DEFAULT ''::text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cargo_cxc_check CHECK ((monto_aplicado <= monto)),
    CONSTRAINT cargo_cxc_estado_check CHECK ((estado = ANY (ARRAY['ABIERTO'::text, 'PARCIAL'::text, 'SALDADO'::text, 'ANULADO'::text]))),
    CONSTRAINT cargo_cxc_monto_aplicado_check CHECK ((monto_aplicado >= (0)::numeric)),
    CONSTRAINT cargo_cxc_monto_check CHECK ((monto > (0)::numeric)),
    CONSTRAINT cargo_cxc_origen_check CHECK ((origen = ANY (ARRAY['GENERADO'::text, 'RECONSTRUIDO'::text, 'SALDO_INICIAL'::text, 'AJUSTE'::text, 'IMPORTADO'::text])))
);


--
-- Name: clasificacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.clasificacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    concepto_id uuid NOT NULL,
    nombre text NOT NULL,
    cuenta_contable_futura text,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    es_contabilidad boolean DEFAULT false NOT NULL
);


--
-- Name: COLUMN clasificacion.es_contabilidad; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.clasificacion.es_contabilidad IS 'Esta clasificación es de Contabilidad: sus facturas no requieren validación de área.';


--
-- Name: cobro_aplicacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cobro_aplicacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    cobro_id uuid NOT NULL,
    cargo_id uuid NOT NULL,
    monto numeric(16,2) NOT NULL,
    parcial boolean DEFAULT false NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cobro_aplicacion_monto_check CHECK ((monto > (0)::numeric))
);


--
-- Name: cobro_cxc; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cobro_cxc (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    contrato_id uuid,
    consecutivo text DEFAULT ''::text NOT NULL,
    fecha_pago date NOT NULL,
    fecha_bancaria date,
    fecha_registro date,
    monto numeric(16,2) NOT NULL,
    saldo_a_favor numeric(16,2) DEFAULT 0 NOT NULL,
    forma_pago_id uuid,
    asociacion_id uuid,
    planilla_id uuid,
    referencia text DEFAULT ''::text NOT NULL,
    concepto_origen text DEFAULT ''::text NOT NULL,
    origen text DEFAULT 'ARCHIVO'::text NOT NULL,
    estado text DEFAULT 'APLICADO'::text NOT NULL,
    idempotency_key text,
    movimiento_bancario_id uuid,
    reversado_por uuid,
    reversado_en timestamp with time zone,
    reversa_motivo text DEFAULT ''::text NOT NULL,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    contrato_origen text DEFAULT ''::text NOT NULL,
    CONSTRAINT cobro_cxc_estado_check CHECK ((estado = ANY (ARRAY['APLICADO'::text, 'SIN_IDENTIFICAR'::text, 'REVERSADO'::text]))),
    CONSTRAINT cobro_cxc_monto_check CHECK ((monto > (0)::numeric)),
    CONSTRAINT cobro_cxc_origen_check CHECK ((origen = ANY (ARRAY['ARCHIVO'::text, 'API'::text, 'CAJA'::text, 'BANCO'::text, 'PLANILLA'::text]))),
    CONSTRAINT cobro_cxc_saldo_a_favor_check CHECK ((saldo_a_favor >= (0)::numeric)),
    CONSTRAINT cobro_cxc_saldo_favor_coherente CHECK ((saldo_a_favor <= monto))
);


--
-- Name: comprobante_pago; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comprobante_pago (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    documento_id uuid NOT NULL,
    filename text NOT NULL,
    mime text DEFAULT 'application/pdf'::text NOT NULL,
    contenido bytea NOT NULL,
    subido_por uuid,
    subido_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: concepto; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.concepto (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    visible_cxp boolean DEFAULT true NOT NULL,
    es_contabilidad boolean DEFAULT false NOT NULL,
    naturaleza text DEFAULT 'NEUTRO'::text NOT NULL,
    naturaleza_declarada boolean DEFAULT false NOT NULL,
    CONSTRAINT concepto_naturaleza_check CHECK ((naturaleza = ANY (ARRAY['INGRESO'::text, 'GASTO'::text, 'NEUTRO'::text])))
);


--
-- Name: COLUMN concepto.es_contabilidad; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.concepto.es_contabilidad IS 'Todo el rubro es de Contabilidad: sus facturas no requieren validación de área.';


--
-- Name: COLUMN concepto.naturaleza; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.concepto.naturaleza IS 'Qué es el concepto para el EBITDA: INGRESO, GASTO o NEUTRO (no cuenta). Lo declara el usuario en el Catálogo.';


--
-- Name: COLUMN concepto.naturaleza_declarada; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.concepto.naturaleza_declarada IS 'true = una persona declaró la naturaleza (aunque haya elegido NEUTRO). false = nadie la declaró y el valor viene del default. Separa la decisión del silencio: sin esto, «no entra al EBITDA a propósito» y «falta decidir» son el mismo dato.';


--
-- Name: concepto_nomina; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.concepto_nomina (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    tipo text NOT NULL,
    afecta_ccss boolean DEFAULT true NOT NULL,
    afecta_renta boolean DEFAULT true NOT NULL,
    afecta_aguinaldo boolean DEFAULT true NOT NULL,
    base_legal text,
    de_sistema boolean DEFAULT false NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    por_horas boolean DEFAULT false NOT NULL,
    CONSTRAINT concepto_nomina_tipo_check CHECK ((tipo = ANY (ARRAY['INGRESO'::text, 'DEDUCCION'::text])))
);


--
-- Name: COLUMN concepto_nomina.por_horas; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.concepto_nomina.por_horas IS 'true = la novedad se captura en HORAS y el monto lo deriva el motor (horas × valor hora × factor, art. 139 CT). false = se captura el monto del período.';


--
-- Name: contrato_cxc; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contrato_cxc (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    numero text NOT NULL,
    sede_id uuid,
    cliente_nombre text DEFAULT ''::text NOT NULL,
    documento text DEFAULT ''::text NOT NULL,
    telefonos text DEFAULT ''::text NOT NULL,
    correos text DEFAULT ''::text NOT NULL,
    servicio text DEFAULT ''::text NOT NULL,
    tipo_servicio text DEFAULT ''::text NOT NULL,
    modalidad_id uuid,
    forma_pago_id uuid,
    asociacion_id uuid,
    dia_pago smallint,
    cuota_vigente numeric(14,2) DEFAULT 0 NOT NULL,
    fecha_inicial date,
    fecha_primer_cobro date,
    tarjeta_vence date,
    estado text DEFAULT 'ACTIVO'::text NOT NULL,
    score_origen integer,
    estado_origen text DEFAULT ''::text NOT NULL,
    morosidad_origen text DEFAULT ''::text NOT NULL,
    dias_vencidos_origen integer,
    saldo_origen numeric(14,2),
    revision_pendiente boolean DEFAULT false NOT NULL,
    revision_motivo text DEFAULT ''::text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT contrato_cxc_cuota_vigente_check CHECK ((cuota_vigente >= (0)::numeric)),
    CONSTRAINT contrato_cxc_dia_pago_check CHECK (((dia_pago IS NULL) OR ((dia_pago >= 1) AND (dia_pago <= 31)))),
    CONSTRAINT contrato_cxc_estado_check CHECK ((estado = ANY (ARRAY['ACTIVO'::text, 'SUSPENDIDO'::text, 'LEGAL'::text, 'CANCELADO'::text])))
);


--
-- Name: corrida_linea; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.corrida_linea (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    corrida_id uuid NOT NULL,
    empleado_id uuid NOT NULL,
    nombre text NOT NULL,
    identificacion text NOT NULL,
    iban text,
    departamento text,
    puesto text,
    salario_base numeric(14,2) NOT NULL,
    hijos integer DEFAULT 0 NOT NULL,
    conyuge boolean DEFAULT false NOT NULL,
    bruto numeric(14,2) DEFAULT 0 NOT NULL,
    base_ccss numeric(14,2) DEFAULT 0 NOT NULL,
    base_renta numeric(14,2) DEFAULT 0 NOT NULL,
    ccss_obrero numeric(14,2) DEFAULT 0 NOT NULL,
    renta numeric(14,2) DEFAULT 0 NOT NULL,
    deducciones numeric(14,2) DEFAULT 0 NOT NULL,
    adelanto numeric(14,2) DEFAULT 0 NOT NULL,
    neto numeric(14,2) DEFAULT 0 NOT NULL,
    patronal numeric(14,2) DEFAULT 0 NOT NULL,
    prov_aguinaldo numeric(14,2) DEFAULT 0 NOT NULL,
    prov_vacaciones numeric(14,2) DEFAULT 0 NOT NULL,
    prov_cesantia numeric(14,2) DEFAULT 0 NOT NULL,
    detalle jsonb DEFAULT '[]'::jsonb NOT NULL,
    tratamiento text DEFAULT 'MENSUAL'::text NOT NULL,
    CONSTRAINT corrida_linea_tratamiento_check CHECK ((tratamiento = ANY (ARRAY['QUINCENA_1'::text, 'QUINCENA_2'::text, 'ADELANTO'::text, 'MENSUAL'::text])))
);


--
-- Name: COLUMN corrida_linea.tratamiento; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.corrida_linea.tratamiento IS 'QUINCENA_1/QUINCENA_2 = salario quincenal real · ADELANTO = anticipo sin deducciones · MENSUAL = liquidación del mes';


--
-- Name: corrida_nomina; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.corrida_nomina (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    anio integer NOT NULL,
    mes integer NOT NULL,
    tipo text NOT NULL,
    estado text DEFAULT 'BORRADOR'::text NOT NULL,
    fecha_pago date NOT NULL,
    parametros jsonb NOT NULL,
    total_bruto numeric(14,2) DEFAULT 0 NOT NULL,
    total_ccss_obrero numeric(14,2) DEFAULT 0 NOT NULL,
    total_renta numeric(14,2) DEFAULT 0 NOT NULL,
    total_deducciones numeric(14,2) DEFAULT 0 NOT NULL,
    total_adelanto numeric(14,2) DEFAULT 0 NOT NULL,
    total_neto numeric(14,2) DEFAULT 0 NOT NULL,
    total_patronal numeric(14,2) DEFAULT 0 NOT NULL,
    total_provisiones numeric(14,2) DEFAULT 0 NOT NULL,
    creado_por uuid,
    aprobado_por uuid,
    pagado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    aprobado_en timestamp with time zone,
    pagado_en timestamp with time zone,
    CONSTRAINT corrida_nomina_anio_check CHECK ((anio >= 2024)),
    CONSTRAINT corrida_nomina_estado_check CHECK ((estado = ANY (ARRAY['BORRADOR'::text, 'APROBADA'::text, 'PAGADA'::text, 'ANULADA'::text]))),
    CONSTRAINT corrida_nomina_mes_check CHECK (((mes >= 1) AND (mes <= 12))),
    CONSTRAINT corrida_nomina_tipo_check CHECK ((tipo = ANY (ARRAY['ADELANTO'::text, 'LIQUIDACION'::text])))
);


--
-- Name: corrida_novedad; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.corrida_novedad (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    corrida_id uuid NOT NULL,
    empleado_id uuid NOT NULL,
    concepto_id uuid NOT NULL,
    monto numeric(14,2) NOT NULL,
    cantidad numeric(8,2),
    CONSTRAINT corrida_novedad_cantidad_check CHECK (((cantidad IS NULL) OR (cantidad > (0)::numeric))),
    CONSTRAINT corrida_novedad_monto_o_cantidad CHECK (((monto > (0)::numeric) OR ((monto = (0)::numeric) AND (cantidad IS NOT NULL) AND (cantidad > (0)::numeric))))
);


--
-- Name: cuenta_bancaria; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cuenta_bancaria (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    banco_id uuid NOT NULL,
    iban text,
    moneda text NOT NULL,
    alias text,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cuenta_bancaria_moneda_check CHECK ((moneda = ANY (ARRAY['CRC'::text, 'USD'::text])))
);


--
-- Name: cxc_asociacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_asociacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    patrono text,
    contacto text,
    correo text,
    activa boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: cxc_canal_gestion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_canal_gestion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    orden smallint DEFAULT 0 NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: cxc_forma_pago; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_forma_pago (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    factor_recuperacion numeric(4,2) DEFAULT 1.00 NOT NULL,
    es_asociacion boolean DEFAULT false NOT NULL,
    es_domiciliado boolean DEFAULT false NOT NULL,
    activa boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cxc_forma_pago_factor_recuperacion_check CHECK (((factor_recuperacion >= 0.10) AND (factor_recuperacion <= 2.00)))
);


--
-- Name: cxc_importacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_importacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    tipo text NOT NULL,
    archivo text DEFAULT ''::text NOT NULL,
    estado text DEFAULT 'PREVISUALIZADA'::text NOT NULL,
    filas integer DEFAULT 0 NOT NULL,
    nuevos integer DEFAULT 0 NOT NULL,
    actualizados integer DEFAULT 0 NOT NULL,
    duplicados integer DEFAULT 0 NOT NULL,
    cuarentena integer DEFAULT 0 NOT NULL,
    reporte jsonb,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    confirmado_en timestamp with time zone,
    CONSTRAINT cxc_importacion_estado_check CHECK ((estado = ANY (ARRAY['PREVISUALIZADA'::text, 'CONFIRMADA'::text, 'DESCARTADA'::text]))),
    CONSTRAINT cxc_importacion_tipo_check CHECK ((tipo = ANY (ARRAY['CONTRATOS'::text, 'COBROS'::text, 'PLANILLA'::text])))
);


--
-- Name: cxc_modalidad; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_modalidad (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    meses_ciclo smallint DEFAULT 1 NOT NULL,
    quincenal boolean DEFAULT false NOT NULL,
    activa boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cxc_modalidad_meses_ciclo_check CHECK (((meses_ciclo >= 1) AND (meses_ciclo <= 12)))
);


--
-- Name: cxc_parametro; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_parametro (
    empresa_id uuid NOT NULL,
    clave text NOT NULL,
    valor text NOT NULL,
    descripcion text DEFAULT ''::text NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_por uuid
);


--
-- Name: cxc_planilla; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_planilla (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    asociacion_id uuid NOT NULL,
    referencia text DEFAULT ''::text NOT NULL,
    periodo text DEFAULT ''::text NOT NULL,
    nota text DEFAULT ''::text NOT NULL,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cxc_planilla_periodo_no_vacio CHECK ((periodo <> ''::text))
);


--
-- Name: cxc_planilla_movimiento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_planilla_movimiento (
    empresa_id uuid NOT NULL,
    planilla_id uuid NOT NULL,
    movimiento_bancario_id uuid NOT NULL,
    vinculado_por uuid,
    vinculado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: cxc_resultado_gestion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_resultado_gestion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    codigo text NOT NULL,
    etiqueta text NOT NULL,
    es_contacto boolean DEFAULT true NOT NULL,
    exige_promesa boolean DEFAULT false NOT NULL,
    dato_malo boolean DEFAULT false NOT NULL,
    orden smallint DEFAULT 0 NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: cxc_sede; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_sede (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    razon_social text,
    plaza text,
    activa boolean DEFAULT true NOT NULL,
    orden integer DEFAULT 0 NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: cxc_suspension; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_suspension (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    contrato_id uuid NOT NULL,
    cuotas_vencidas integer NOT NULL,
    saldo_al_suspender numeric(14,2) DEFAULT 0 NOT NULL,
    motivo text NOT NULL,
    suspendido_por uuid,
    suspendido_en timestamp with time zone DEFAULT now() NOT NULL,
    reactivado_por uuid,
    reactivado_en timestamp with time zone,
    reactivacion_motivo text DEFAULT ''::text NOT NULL,
    meses_mora numeric(6,1) DEFAULT 0 NOT NULL
);


--
-- Name: cxc_tramo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_tramo (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    codigo text NOT NULL,
    etiqueta text NOT NULL,
    dias_min integer NOT NULL,
    dias_max integer NOT NULL,
    orden smallint NOT NULL,
    prob_recuperacion numeric(4,2) NOT NULL,
    estrategia text DEFAULT ''::text NOT NULL,
    canal_sugerido text DEFAULT ''::text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT cxc_tramo_check CHECK ((dias_min <= dias_max)),
    CONSTRAINT cxc_tramo_prob_recuperacion_check CHECK (((prob_recuperacion >= (0)::numeric) AND (prob_recuperacion <= (1)::numeric)))
);


--
-- Name: cxc_usuario_sede; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxc_usuario_sede (
    empresa_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    sede_id uuid NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: cxp_parametro; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cxp_parametro (
    empresa_id uuid NOT NULL,
    clave text NOT NULL,
    valor text NOT NULL,
    descripcion text DEFAULT ''::text NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_por uuid
);


--
-- Name: deduccion_empleado; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deduccion_empleado (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    empleado_id uuid NOT NULL,
    concepto_id uuid NOT NULL,
    etiqueta text NOT NULL,
    cuota numeric(14,2) NOT NULL,
    saldo_total numeric(14,2),
    saldo_restante numeric(14,2),
    prioridad integer DEFAULT 100 NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    frecuencia text DEFAULT 'MENSUAL'::text NOT NULL,
    CONSTRAINT deduccion_empleado_cuota_check CHECK ((cuota > (0)::numeric)),
    CONSTRAINT deduccion_empleado_frecuencia_check CHECK ((frecuencia = ANY (ARRAY['AMBAS'::text, 'PRIMERA'::text, 'SEGUNDA'::text, 'MENSUAL'::text])))
);


--
-- Name: COLUMN deduccion_empleado.frecuencia; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.deduccion_empleado.frecuencia IS 'AMBAS = cada quincena · PRIMERA/SEGUNDA = solo esa quincena · MENSUAL = una vez al mes (en la 2ª)';


--
-- Name: departamento; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.departamento (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    codigo text,
    centro_costo text,
    activo boolean DEFAULT true NOT NULL,
    orden integer DEFAULT 0 NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: departamento_validador; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.departamento_validador (
    departamento_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    rol text DEFAULT 'TITULAR'::text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT departamento_validador_rol_check CHECK ((rol = ANY (ARRAY['TITULAR'::text, 'SUPLENTE'::text])))
);


--
-- Name: documento_cxp; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.documento_cxp (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    proveedor_id uuid NOT NULL,
    clave text NOT NULL,
    consecutivo text,
    fecha_emision date NOT NULL,
    moneda text DEFAULT 'CRC'::text NOT NULL,
    subtotal numeric(16,2) DEFAULT 0 NOT NULL,
    iva numeric(16,2) DEFAULT 0 NOT NULL,
    retencion numeric(16,2) DEFAULT 0 NOT NULL,
    total numeric(16,2) DEFAULT 0 NOT NULL,
    tc_aplicado numeric(14,4),
    total_crc numeric(16,2) DEFAULT 0 NOT NULL,
    descripcion text,
    estado text DEFAULT 'RECIBIDO'::text NOT NULL,
    fecha_pago_programada date,
    huella text,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    concepto_id uuid,
    clasificacion_id uuid,
    fecha_vencimiento date,
    tipo text DEFAULT 'CXP'::text NOT NULL,
    subclasificacion_id uuid,
    lote_id uuid,
    comprobante_enviado_en timestamp with time zone,
    clasif_auto boolean DEFAULT false NOT NULL,
    prioridad text DEFAULT ''::text NOT NULL,
    nota_revision text,
    departamento_id uuid,
    validado_depto_por uuid,
    validado_depto_en timestamp with time zone,
    validacion_respaldo text,
    validacion_nota text,
    es_contabilidad boolean,
    contabilidad_motivo text,
    contabilidad_marcado_por uuid,
    contabilidad_marcado_en timestamp with time zone,
    requiere_validacion boolean,
    validacion_motivo text,
    CONSTRAINT documento_cxp_estado_check CHECK ((estado = ANY (ARRAY['RECIBIDO'::text, 'REVISADO'::text, 'VALIDADO_DEPTO'::text, 'APROBADO'::text, 'PROGRAMADO'::text, 'PAGADO'::text, 'CONCILIADO'::text, 'DENEGADO'::text, 'ANULADO'::text, 'LIQUIDADA'::text, 'REBOTADA'::text]))),
    CONSTRAINT documento_cxp_moneda_check CHECK ((moneda = ANY (ARRAY['CRC'::text, 'USD'::text]))),
    CONSTRAINT documento_cxp_prioridad_check CHECK ((prioridad = ANY (ARRAY[''::text, 'A'::text, 'AA'::text]))),
    CONSTRAINT documento_cxp_tipo_check CHECK ((tipo = ANY (ARRAY['CXP'::text, 'ANTICIPO'::text, 'VIATICOS'::text, 'REINTEGRO'::text, 'INTERNO'::text])))
);


--
-- Name: COLUMN documento_cxp.es_contabilidad; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.documento_cxp.es_contabilidad IS 'Override de la marca: NULL hereda de proveedor/concepto/clasificación, true fuerza «de Contabilidad», false fuerza que la valide el área.';


--
-- Name: COLUMN documento_cxp.requiere_validacion; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.documento_cxp.requiere_validacion IS 'Si el área tiene que confirmar la conformidad. NULL = todavía no evaluado (se evalúa al revisar).';


--
-- Name: COLUMN documento_cxp.validacion_motivo; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.documento_cxp.validacion_motivo IS 'Por qué requiere validación: MONTO, PROVEEDOR_NUEVO o DESVIO. Vacío cuando no la requiere.';


--
-- Name: documento_cxp_aprobacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.documento_cxp_aprobacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    documento_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    rol text NOT NULL,
    aprobado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: empleado; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.empleado (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    tipo_identificacion text DEFAULT 'CEDULA'::text NOT NULL,
    identificacion text NOT NULL,
    email text,
    telefono text,
    iban text,
    departamento_id uuid,
    puesto text,
    fecha_ingreso date NOT NULL,
    fecha_salida date,
    salario_base numeric(14,2) NOT NULL,
    jornada text DEFAULT 'MENSUAL'::text NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    hijos integer DEFAULT 0 NOT NULL,
    conyuge boolean DEFAULT false NOT NULL,
    CONSTRAINT empleado_hijos_check CHECK (((hijos >= 0) AND (hijos <= 20))),
    CONSTRAINT empleado_jornada_check CHECK ((jornada = ANY (ARRAY['MENSUAL'::text, 'QUINCENAL'::text, 'SEMANAL'::text, 'HORAS'::text]))),
    CONSTRAINT empleado_salario_base_check CHECK ((salario_base >= (0)::numeric)),
    CONSTRAINT empleado_tipo_identificacion_check CHECK ((tipo_identificacion = ANY (ARRAY['CEDULA'::text, 'DIMEX'::text, 'PASAPORTE'::text])))
);


--
-- Name: empresa; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.empresa (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    nombre text NOT NULL,
    tipo_legal text,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    tolerancia_traslado numeric(6,4) DEFAULT 0.01 NOT NULL
);


--
-- Name: finiquito; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.finiquito (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    empleado_id uuid NOT NULL,
    motivo text NOT NULL,
    fecha_salida date NOT NULL,
    estado text DEFAULT 'BORRADOR'::text NOT NULL,
    dias_vacaciones numeric(6,2) DEFAULT 0 NOT NULL,
    salario_promedio numeric(14,2) DEFAULT 0 NOT NULL,
    salario_diario numeric(14,2) DEFAULT 0 NOT NULL,
    anios_servicio integer DEFAULT 0 NOT NULL,
    preaviso numeric(14,2) DEFAULT 0 NOT NULL,
    cesantia numeric(14,2) DEFAULT 0 NOT NULL,
    vacaciones numeric(14,2) DEFAULT 0 NOT NULL,
    aguinaldo numeric(14,2) DEFAULT 0 NOT NULL,
    descuentos numeric(14,2) DEFAULT 0 NOT NULL,
    total numeric(14,2) DEFAULT 0 NOT NULL,
    detalle jsonb DEFAULT '[]'::jsonb NOT NULL,
    creado_por uuid,
    aprobado_por uuid,
    pagado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    aprobado_en timestamp with time zone,
    pagado_en timestamp with time zone,
    base_ccss numeric(14,2) DEFAULT 0 NOT NULL,
    ccss_obrero numeric(14,2) DEFAULT 0 NOT NULL,
    renta numeric(14,2) DEFAULT 0 NOT NULL,
    dias_vacaciones_manual boolean DEFAULT true NOT NULL,
    patronal numeric(14,2) DEFAULT 0 NOT NULL,
    CONSTRAINT finiquito_dias_vacaciones_check CHECK ((dias_vacaciones >= (0)::numeric)),
    CONSTRAINT finiquito_estado_check CHECK ((estado = ANY (ARRAY['BORRADOR'::text, 'APROBADO'::text, 'PAGADO'::text, 'ANULADO'::text]))),
    CONSTRAINT finiquito_motivo_check CHECK ((motivo = ANY (ARRAY['DESPIDO_RESPONSABILIDAD'::text, 'RENUNCIA'::text, 'FIN_CONTRATO'::text, 'MUTUO_ACUERDO'::text])))
);


--
-- Name: COLUMN finiquito.dias_vacaciones_manual; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.finiquito.dias_vacaciones_manual IS 'true = los días los digitó RRHH y se respetan · false = vienen del saldo y se recalculan al aprobar';


--
-- Name: gestion_cxc; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gestion_cxc (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    contrato_id uuid NOT NULL,
    usuario_id uuid,
    canal_id uuid NOT NULL,
    resultado_id uuid NOT NULL,
    notas text DEFAULT ''::text NOT NULL,
    saldo_al_gestionar numeric(14,2) DEFAULT 0 NOT NULL,
    dias_mora_al_gestionar integer DEFAULT 0 NOT NULL,
    tramo_al_gestionar text DEFAULT ''::text NOT NULL,
    gestionada_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: importacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.importacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    cuenta_bancaria_id uuid NOT NULL,
    source_file_hash text NOT NULL,
    nombre_archivo text NOT NULL,
    estado text DEFAULT 'CARGADA'::text NOT NULL,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    archivo bytea,
    banco text,
    CONSTRAINT importacion_estado_check CHECK ((estado = ANY (ARRAY['CARGADA'::text, 'PREVISUALIZADA'::text, 'CONFIRMADA'::text, 'CERRADA'::text])))
);


--
-- Name: incapacidad; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.incapacidad (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    empleado_id uuid NOT NULL,
    entidad text NOT NULL,
    fecha_inicio date NOT NULL,
    dias integer NOT NULL,
    boleta text,
    observaciones text,
    anulada boolean DEFAULT false NOT NULL,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT incapacidad_dias_check CHECK (((dias > 0) AND (dias <= 365))),
    CONSTRAINT incapacidad_entidad_check CHECK ((entidad = ANY (ARRAY['CCSS'::text, 'INS'::text])))
);


--
-- Name: lote_pago; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.lote_pago (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    numero bigint NOT NULL,
    fecha_corte date NOT NULL,
    estado text DEFAULT 'ABIERTO'::text NOT NULL,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT lote_pago_estado_check CHECK ((estado = ANY (ARRAY['ABIERTO'::text, 'CERRADO'::text])))
);


--
-- Name: movimiento_bancario; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.movimiento_bancario (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    cuenta_bancaria_id uuid NOT NULL,
    importacion_id uuid,
    fecha date NOT NULL,
    documento text,
    descripcion text,
    debito numeric(16,2) DEFAULT 0 NOT NULL,
    credito numeric(16,2) DEFAULT 0 NOT NULL,
    moneda_original text DEFAULT 'CRC'::text NOT NULL,
    monto_original numeric(16,2) DEFAULT 0 NOT NULL,
    monto_crc numeric(16,2) DEFAULT 0 NOT NULL,
    tc_aplicado numeric(14,4),
    concepto_id uuid,
    clasificacion_id uuid,
    estado_clasificacion text DEFAULT 'NO_IDENTIFICADO'::text NOT NULL,
    confianza numeric(5,2),
    es_traslado boolean DEFAULT false NOT NULL,
    par_traslado_id uuid,
    natural_key text NOT NULL,
    indice_ocurrencia integer DEFAULT 1 NOT NULL,
    incluido boolean DEFAULT true NOT NULL,
    origen_historico boolean DEFAULT false NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    documento_cxp_id uuid,
    CONSTRAINT movimiento_bancario_estado_clasificacion_check CHECK ((estado_clasificacion = ANY (ARRAY['NO_IDENTIFICADO'::text, 'AUTO'::text, 'REVISADO'::text]))),
    CONSTRAINT movimiento_bancario_moneda_original_check CHECK ((moneda_original = ANY (ARRAY['CRC'::text, 'USD'::text])))
);


--
-- Name: nomina_archivo_pago; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nomina_archivo_pago (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    corrida_id uuid NOT NULL,
    consecutivo integer NOT NULL,
    registros integer NOT NULL,
    total numeric(14,2) NOT NULL,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: nomina_parametros; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nomina_parametros (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    anio integer NOT NULL,
    cargas jsonb NOT NULL,
    tramos_renta jsonb NOT NULL,
    ins_riesgos_pct numeric(6,3) DEFAULT 1.000 NOT NULL,
    aplica_ina boolean DEFAULT true NOT NULL,
    adelanto_pct numeric(5,2) DEFAULT 50 NOT NULL,
    adelanto_base text DEFAULT 'SALARIO_BASE'::text NOT NULL,
    redondeo text DEFAULT 'COLON'::text NOT NULL,
    provision_base text DEFAULT 'REMUNERACION_TOTAL'::text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    aguinaldo_pct numeric(6,3) DEFAULT 8.33 NOT NULL,
    vacaciones_pct numeric(6,3) DEFAULT 4.16 NOT NULL,
    cesantia_pct numeric(6,3) DEFAULT 1.50 NOT NULL,
    vacaciones_dias_mes numeric(5,2) DEFAULT 1.00 NOT NULL,
    horas_jornada_mes numeric(6,2) DEFAULT 240 NOT NULL,
    factor_hora_extra numeric(4,2) DEFAULT 1.5 NOT NULL,
    CONSTRAINT nomina_parametros_adelanto_base_check CHECK ((adelanto_base = ANY (ARRAY['SALARIO_BASE'::text, 'BRUTO'::text]))),
    CONSTRAINT nomina_parametros_adelanto_pct_check CHECK (((adelanto_pct >= (0)::numeric) AND (adelanto_pct <= (100)::numeric))),
    CONSTRAINT nomina_parametros_anio_check CHECK ((anio >= 2024)),
    CONSTRAINT nomina_parametros_factor_hora_extra_check CHECK ((factor_hora_extra >= 1.5)),
    CONSTRAINT nomina_parametros_horas_jornada_mes_check CHECK (((horas_jornada_mes >= (160)::numeric) AND (horas_jornada_mes <= (260)::numeric))),
    CONSTRAINT nomina_parametros_provision_base_check CHECK ((provision_base = ANY (ARRAY['REMUNERACION_TOTAL'::text, 'SALARIO_BASE'::text]))),
    CONSTRAINT nomina_parametros_redondeo_check CHECK ((redondeo = ANY (ARRAY['COLON'::text, 'CENTIMO'::text])))
);


--
-- Name: COLUMN nomina_parametros.vacaciones_dias_mes; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.nomina_parametros.vacaciones_dias_mes IS 'Días de vacaciones que acumula el empleado por cada mes trabajado (CT art. 153: 1 = 12 días hábiles/año)';


--
-- Name: nota_credito_aplicacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nota_credito_aplicacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nota_id uuid NOT NULL,
    cargo_id uuid NOT NULL,
    monto numeric(16,2) NOT NULL,
    parcial boolean DEFAULT false NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT nota_credito_aplicacion_monto_check CHECK ((monto > (0)::numeric))
);


--
-- Name: nota_credito_cxc; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nota_credito_cxc (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    contrato_id uuid NOT NULL,
    cargo_id uuid,
    fecha date NOT NULL,
    monto numeric(16,2) NOT NULL,
    motivo text NOT NULL,
    estado text DEFAULT 'APLICADA'::text NOT NULL,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    consecutivo bigint,
    anulada_por uuid,
    anulada_en timestamp with time zone,
    anulacion_motivo text DEFAULT ''::text NOT NULL,
    CONSTRAINT nota_credito_cxc_estado_check CHECK ((estado = ANY (ARRAY['APLICADA'::text, 'ANULADA'::text]))),
    CONSTRAINT nota_credito_cxc_monto_check CHECK ((monto > (0)::numeric))
);


--
-- Name: palabra_clave; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.palabra_clave (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    regla_id uuid NOT NULL,
    texto text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: partida_conciliacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.partida_conciliacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    cuenta_bancaria_id uuid NOT NULL,
    anio integer NOT NULL,
    mes integer NOT NULL,
    tipo text NOT NULL,
    descripcion text NOT NULL,
    monto numeric(16,2) NOT NULL,
    signo smallint NOT NULL,
    anulada boolean DEFAULT false NOT NULL,
    registrado_por uuid,
    registrado_en timestamp with time zone DEFAULT now() NOT NULL,
    anulada_por uuid,
    anulada_en timestamp with time zone,
    CONSTRAINT partida_conciliacion_anio_check CHECK ((anio >= 2024)),
    CONSTRAINT partida_conciliacion_mes_check CHECK (((mes >= 1) AND (mes <= 12))),
    CONSTRAINT partida_conciliacion_monto_check CHECK ((monto > (0)::numeric)),
    CONSTRAINT partida_conciliacion_signo_check CHECK ((signo = ANY (ARRAY['-1'::integer, 1]))),
    CONSTRAINT partida_conciliacion_tipo_check CHECK ((tipo = ANY (ARRAY['DEPOSITO_NO_ACREDITADO'::text, 'TRANSFERENCIA_NO_PRESENTADA'::text, 'CARGO_BANCO_NO_REGISTRADO'::text, 'ABONO_BANCO_NO_REGISTRADO'::text, 'OTRA'::text])))
);


--
-- Name: periodo_cierre; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.periodo_cierre (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    anio integer NOT NULL,
    mes integer NOT NULL,
    no_identificados_al_cierre integer DEFAULT 0 NOT NULL,
    cerrado_por uuid,
    cerrado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT periodo_cierre_mes_check CHECK (((mes >= 1) AND (mes <= 12)))
);


--
-- Name: permiso; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.permiso (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    codigo text NOT NULL,
    modulo text NOT NULL,
    nombre text NOT NULL,
    descripcion text,
    critico boolean DEFAULT false NOT NULL,
    orden integer DEFAULT 0 NOT NULL
);


--
-- Name: plantilla_correo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plantilla_correo (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    clave text NOT NULL,
    asunto text NOT NULL,
    cuerpo text NOT NULL,
    actualizado_por uuid,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: promesa_pago_cxc; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.promesa_pago_cxc (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    gestion_id uuid NOT NULL,
    contrato_id uuid NOT NULL,
    fecha_promesa date NOT NULL,
    monto numeric(14,2),
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT promesa_pago_cxc_monto_check CHECK (((monto IS NULL) OR (monto > (0)::numeric)))
);


--
-- Name: proveedor; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.proveedor (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    tipo_identificacion text,
    identificacion text,
    email text,
    telefono text,
    iban text,
    retencion_renta_pct numeric(5,2) DEFAULT 0 NOT NULL,
    exento_iva boolean DEFAULT false NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    gasto_concepto_id uuid,
    gasto_clasificacion_id uuid,
    gasto_subclasificacion_id uuid,
    condicion_pago text DEFAULT 'CONTADO'::text NOT NULL,
    plazo_credito_dias integer DEFAULT 0 NOT NULL,
    departamento text,
    es_contabilidad boolean DEFAULT false NOT NULL,
    CONSTRAINT proveedor_condicion_pago_check CHECK ((condicion_pago = ANY (ARRAY['CONTADO'::text, 'CREDITO'::text]))),
    CONSTRAINT proveedor_tipo_identificacion_check CHECK ((tipo_identificacion = ANY (ARRAY['FISICA'::text, 'JURIDICA'::text, 'DIMEX'::text, 'NITE'::text])))
);


--
-- Name: COLUMN proveedor.es_contabilidad; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.proveedor.es_contabilidad IS 'Las facturas de este proveedor son de Contabilidad: no requieren validación de área.';


--
-- Name: proveedor_gasto; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.proveedor_gasto (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    proveedor_id uuid NOT NULL,
    concepto_id uuid NOT NULL,
    clasificacion_id uuid,
    subclasificacion_id uuid,
    usos integer DEFAULT 1 NOT NULL,
    ultimo_uso timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: proyeccion_escenario; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.proyeccion_escenario (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    periodo text NOT NULL,
    metodo text NOT NULL,
    metodo_efectivo text NOT NULL,
    meta_crecimiento_pct numeric(6,2) DEFAULT 0 NOT NULL,
    lineas_ingreso text[] DEFAULT '{}'::text[] NOT NULL,
    dia_calculo integer NOT NULL,
    real_acumulado numeric(18,2) NOT NULL,
    cierre_proyectado numeric(18,2) NOT NULL,
    meta_monto numeric(18,2) NOT NULL,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT proyeccion_escenario_metodo_check CHECK ((metodo = ANY (ARRAY['RITMO'::text, 'HISTORICO'::text, 'PROMEDIO'::text, 'COINCIDENCIA'::text])))
);


--
-- Name: regla_clasificacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.regla_clasificacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    nombre text NOT NULL,
    aplica_a text NOT NULL,
    concepto_id uuid NOT NULL,
    clasificacion_id uuid NOT NULL,
    prioridad integer DEFAULT 100 NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    aciertos integer DEFAULT 0 NOT NULL,
    CONSTRAINT regla_clasificacion_aplica_a_check CHECK ((aplica_a = ANY (ARRAY['DEBITO'::text, 'CREDITO'::text, 'MIXTO'::text])))
);


--
-- Name: rol; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rol (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    codigo text NOT NULL,
    nombre text NOT NULL,
    descripcion text,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    empresa_id uuid
);


--
-- Name: rol_permiso; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.rol_permiso (
    empresa_id uuid NOT NULL,
    rol_id uuid NOT NULL,
    permiso_id uuid NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: saldo_cuenta_diario; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.saldo_cuenta_diario (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    cuenta_bancaria_id uuid NOT NULL,
    fecha date NOT NULL,
    saldo numeric(16,2) NOT NULL,
    nota text,
    capturado_por uuid,
    capturado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    revisado_por uuid,
    revisado_en timestamp with time zone
);


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version bigint NOT NULL,
    dirty boolean NOT NULL
);


--
-- Name: sesion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sesion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    usuario_id uuid NOT NULL,
    token_hash text NOT NULL,
    expira_en timestamp with time zone NOT NULL,
    revocado boolean DEFAULT false NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: subclasificacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subclasificacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    clasificacion_id uuid NOT NULL,
    nombre text NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: tipo_cambio_cotizacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tipo_cambio_cotizacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    fecha date NOT NULL,
    valor numeric(14,4) NOT NULL,
    fuente text NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tipo_cambio_cotizacion_fuente_check CHECK ((fuente = ANY (ARRAY['BCCR'::text, 'MANUAL'::text])))
);


--
-- Name: tipo_cambio_mes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tipo_cambio_mes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    anio integer NOT NULL,
    mes integer NOT NULL,
    valor_congelado numeric(14,4),
    estado text DEFAULT 'PROVISIONAL'::text NOT NULL,
    congelado_en timestamp with time zone,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tipo_cambio_mes_estado_check CHECK ((estado = ANY (ARRAY['PROVISIONAL'::text, 'CONGELADO'::text]))),
    CONSTRAINT tipo_cambio_mes_mes_check CHECK (((mes >= 1) AND (mes <= 12)))
);


--
-- Name: usuario; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.usuario (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    nombre text NOT NULL,
    email text NOT NULL,
    password_hash text NOT NULL,
    activo boolean DEFAULT true NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    actualizado_en timestamp with time zone DEFAULT now() NOT NULL,
    debe_cambiar_password boolean DEFAULT false NOT NULL
);


--
-- Name: usuario_empresa_rol; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.usuario_empresa_rol (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    usuario_id uuid NOT NULL,
    rol_id uuid NOT NULL,
    creado_en timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: vacacion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vacacion (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    empresa_id uuid NOT NULL,
    empleado_id uuid NOT NULL,
    fecha_inicio date NOT NULL,
    dias numeric(6,2) NOT NULL,
    observaciones text,
    anulada boolean DEFAULT false NOT NULL,
    creado_por uuid,
    creado_en timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT vacacion_dias_check CHECK (((dias > (0)::numeric) AND (dias <= (365)::numeric)))
);


--
-- Name: acta_conciliacion acta_conciliacion_empresa_id_cuenta_bancaria_id_anio_mes_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acta_conciliacion
    ADD CONSTRAINT acta_conciliacion_empresa_id_cuenta_bancaria_id_anio_mes_key UNIQUE (empresa_id, cuenta_bancaria_id, anio, mes);


--
-- Name: acta_conciliacion acta_conciliacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acta_conciliacion
    ADD CONSTRAINT acta_conciliacion_pkey PRIMARY KEY (id);


--
-- Name: anticipo_aplicacion anticipo_aplicacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anticipo_aplicacion
    ADD CONSTRAINT anticipo_aplicacion_pkey PRIMARY KEY (id);


--
-- Name: arreglo_cuota_cxc arreglo_cuota_cxc_arreglo_id_numero_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_cuota_cxc
    ADD CONSTRAINT arreglo_cuota_cxc_arreglo_id_numero_key UNIQUE (arreglo_id, numero);


--
-- Name: arreglo_cuota_cxc arreglo_cuota_cxc_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_cuota_cxc
    ADD CONSTRAINT arreglo_cuota_cxc_pkey PRIMARY KEY (id);


--
-- Name: arreglo_pago_cxc arreglo_pago_cxc_empresa_id_consecutivo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_pago_cxc
    ADD CONSTRAINT arreglo_pago_cxc_empresa_id_consecutivo_key UNIQUE (empresa_id, consecutivo);


--
-- Name: arreglo_pago_cxc arreglo_pago_cxc_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_pago_cxc
    ADD CONSTRAINT arreglo_pago_cxc_pkey PRIMARY KEY (id);


--
-- Name: auditoria_evento auditoria_evento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auditoria_evento
    ADD CONSTRAINT auditoria_evento_pkey PRIMARY KEY (id);


--
-- Name: banco banco_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.banco
    ADD CONSTRAINT banco_pkey PRIMARY KEY (id);


--
-- Name: bccr_sync_log bccr_sync_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bccr_sync_log
    ADD CONSTRAINT bccr_sync_log_pkey PRIMARY KEY (id);


--
-- Name: caja_chica_fondo caja_chica_fondo_empresa_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_fondo
    ADD CONSTRAINT caja_chica_fondo_empresa_id_nombre_key UNIQUE (empresa_id, nombre);


--
-- Name: caja_chica_fondo caja_chica_fondo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_fondo
    ADD CONSTRAINT caja_chica_fondo_pkey PRIMARY KEY (id);


--
-- Name: caja_chica_vale caja_chica_vale_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_vale
    ADD CONSTRAINT caja_chica_vale_pkey PRIMARY KEY (id);


--
-- Name: cargo_cxc cargo_cxc_contrato_id_periodo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cargo_cxc
    ADD CONSTRAINT cargo_cxc_contrato_id_periodo_key UNIQUE (contrato_id, periodo);


--
-- Name: cargo_cxc cargo_cxc_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cargo_cxc
    ADD CONSTRAINT cargo_cxc_pkey PRIMARY KEY (id);


--
-- Name: clasificacion clasificacion_empresa_id_concepto_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.clasificacion
    ADD CONSTRAINT clasificacion_empresa_id_concepto_id_nombre_key UNIQUE (empresa_id, concepto_id, nombre);


--
-- Name: clasificacion clasificacion_id_concepto_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.clasificacion
    ADD CONSTRAINT clasificacion_id_concepto_id_key UNIQUE (id, concepto_id);


--
-- Name: clasificacion clasificacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.clasificacion
    ADD CONSTRAINT clasificacion_pkey PRIMARY KEY (id);


--
-- Name: cobro_aplicacion cobro_aplicacion_cobro_id_cargo_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_aplicacion
    ADD CONSTRAINT cobro_aplicacion_cobro_id_cargo_id_key UNIQUE (cobro_id, cargo_id);


--
-- Name: cobro_aplicacion cobro_aplicacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_aplicacion
    ADD CONSTRAINT cobro_aplicacion_pkey PRIMARY KEY (id);


--
-- Name: cobro_cxc cobro_cxc_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_cxc
    ADD CONSTRAINT cobro_cxc_pkey PRIMARY KEY (id);


--
-- Name: comprobante_pago comprobante_pago_documento_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comprobante_pago
    ADD CONSTRAINT comprobante_pago_documento_id_key UNIQUE (documento_id);


--
-- Name: comprobante_pago comprobante_pago_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comprobante_pago
    ADD CONSTRAINT comprobante_pago_pkey PRIMARY KEY (id);


--
-- Name: concepto concepto_empresa_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.concepto
    ADD CONSTRAINT concepto_empresa_id_nombre_key UNIQUE (empresa_id, nombre);


--
-- Name: concepto_nomina concepto_nomina_empresa_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.concepto_nomina
    ADD CONSTRAINT concepto_nomina_empresa_id_nombre_key UNIQUE (empresa_id, nombre);


--
-- Name: concepto_nomina concepto_nomina_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.concepto_nomina
    ADD CONSTRAINT concepto_nomina_pkey PRIMARY KEY (id);


--
-- Name: concepto concepto_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.concepto
    ADD CONSTRAINT concepto_pkey PRIMARY KEY (id);


--
-- Name: contrato_cxc contrato_cxc_empresa_id_numero_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_cxc
    ADD CONSTRAINT contrato_cxc_empresa_id_numero_key UNIQUE (empresa_id, numero);


--
-- Name: contrato_cxc contrato_cxc_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_cxc
    ADD CONSTRAINT contrato_cxc_pkey PRIMARY KEY (id);


--
-- Name: corrida_linea corrida_linea_corrida_id_empleado_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_linea
    ADD CONSTRAINT corrida_linea_corrida_id_empleado_id_key UNIQUE (corrida_id, empleado_id);


--
-- Name: corrida_linea corrida_linea_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_linea
    ADD CONSTRAINT corrida_linea_pkey PRIMARY KEY (id);


--
-- Name: corrida_nomina corrida_nomina_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_nomina
    ADD CONSTRAINT corrida_nomina_pkey PRIMARY KEY (id);


--
-- Name: corrida_novedad corrida_novedad_corrida_id_empleado_id_concepto_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_novedad
    ADD CONSTRAINT corrida_novedad_corrida_id_empleado_id_concepto_id_key UNIQUE (corrida_id, empleado_id, concepto_id);


--
-- Name: corrida_novedad corrida_novedad_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_novedad
    ADD CONSTRAINT corrida_novedad_pkey PRIMARY KEY (id);


--
-- Name: cuenta_bancaria cuenta_bancaria_empresa_id_iban_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_bancaria
    ADD CONSTRAINT cuenta_bancaria_empresa_id_iban_key UNIQUE (empresa_id, iban);


--
-- Name: cuenta_bancaria cuenta_bancaria_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_bancaria
    ADD CONSTRAINT cuenta_bancaria_pkey PRIMARY KEY (id);


--
-- Name: cxc_asociacion cxc_asociacion_empresa_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_asociacion
    ADD CONSTRAINT cxc_asociacion_empresa_id_nombre_key UNIQUE (empresa_id, nombre);


--
-- Name: cxc_asociacion cxc_asociacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_asociacion
    ADD CONSTRAINT cxc_asociacion_pkey PRIMARY KEY (id);


--
-- Name: cxc_canal_gestion cxc_canal_gestion_empresa_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_canal_gestion
    ADD CONSTRAINT cxc_canal_gestion_empresa_id_nombre_key UNIQUE (empresa_id, nombre);


--
-- Name: cxc_canal_gestion cxc_canal_gestion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_canal_gestion
    ADD CONSTRAINT cxc_canal_gestion_pkey PRIMARY KEY (id);


--
-- Name: cxc_forma_pago cxc_forma_pago_empresa_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_forma_pago
    ADD CONSTRAINT cxc_forma_pago_empresa_id_nombre_key UNIQUE (empresa_id, nombre);


--
-- Name: cxc_forma_pago cxc_forma_pago_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_forma_pago
    ADD CONSTRAINT cxc_forma_pago_pkey PRIMARY KEY (id);


--
-- Name: cxc_importacion cxc_importacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_importacion
    ADD CONSTRAINT cxc_importacion_pkey PRIMARY KEY (id);


--
-- Name: cxc_modalidad cxc_modalidad_empresa_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_modalidad
    ADD CONSTRAINT cxc_modalidad_empresa_id_nombre_key UNIQUE (empresa_id, nombre);


--
-- Name: cxc_modalidad cxc_modalidad_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_modalidad
    ADD CONSTRAINT cxc_modalidad_pkey PRIMARY KEY (id);


--
-- Name: cxc_parametro cxc_parametro_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_parametro
    ADD CONSTRAINT cxc_parametro_pkey PRIMARY KEY (empresa_id, clave);


--
-- Name: cxc_planilla_movimiento cxc_planilla_movimiento_movimiento_bancario_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla_movimiento
    ADD CONSTRAINT cxc_planilla_movimiento_movimiento_bancario_id_key UNIQUE (movimiento_bancario_id);


--
-- Name: cxc_planilla_movimiento cxc_planilla_movimiento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla_movimiento
    ADD CONSTRAINT cxc_planilla_movimiento_pkey PRIMARY KEY (planilla_id, movimiento_bancario_id);


--
-- Name: cxc_planilla cxc_planilla_periodo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla
    ADD CONSTRAINT cxc_planilla_periodo_key UNIQUE (empresa_id, asociacion_id, periodo);


--
-- Name: cxc_planilla cxc_planilla_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla
    ADD CONSTRAINT cxc_planilla_pkey PRIMARY KEY (id);


--
-- Name: cxc_resultado_gestion cxc_resultado_gestion_empresa_id_codigo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_resultado_gestion
    ADD CONSTRAINT cxc_resultado_gestion_empresa_id_codigo_key UNIQUE (empresa_id, codigo);


--
-- Name: cxc_resultado_gestion cxc_resultado_gestion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_resultado_gestion
    ADD CONSTRAINT cxc_resultado_gestion_pkey PRIMARY KEY (id);


--
-- Name: cxc_sede cxc_sede_empresa_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_sede
    ADD CONSTRAINT cxc_sede_empresa_id_nombre_key UNIQUE (empresa_id, nombre);


--
-- Name: cxc_sede cxc_sede_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_sede
    ADD CONSTRAINT cxc_sede_pkey PRIMARY KEY (id);


--
-- Name: cxc_suspension cxc_suspension_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_suspension
    ADD CONSTRAINT cxc_suspension_pkey PRIMARY KEY (id);


--
-- Name: cxc_tramo cxc_tramo_empresa_id_codigo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_tramo
    ADD CONSTRAINT cxc_tramo_empresa_id_codigo_key UNIQUE (empresa_id, codigo);


--
-- Name: cxc_tramo cxc_tramo_empresa_id_int4range_excl; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_tramo
    ADD CONSTRAINT cxc_tramo_empresa_id_int4range_excl EXCLUDE USING gist (empresa_id WITH =, int4range(dias_min, dias_max, '[]'::text) WITH &&);


--
-- Name: cxc_tramo cxc_tramo_empresa_id_orden_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_tramo
    ADD CONSTRAINT cxc_tramo_empresa_id_orden_key UNIQUE (empresa_id, orden);


--
-- Name: cxc_tramo cxc_tramo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_tramo
    ADD CONSTRAINT cxc_tramo_pkey PRIMARY KEY (id);


--
-- Name: cxc_usuario_sede cxc_usuario_sede_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_usuario_sede
    ADD CONSTRAINT cxc_usuario_sede_pkey PRIMARY KEY (empresa_id, usuario_id, sede_id);


--
-- Name: cxp_parametro cxp_parametro_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxp_parametro
    ADD CONSTRAINT cxp_parametro_pkey PRIMARY KEY (empresa_id, clave);


--
-- Name: deduccion_empleado deduccion_empleado_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deduccion_empleado
    ADD CONSTRAINT deduccion_empleado_pkey PRIMARY KEY (id);


--
-- Name: departamento departamento_empresa_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departamento
    ADD CONSTRAINT departamento_empresa_id_nombre_key UNIQUE (empresa_id, nombre);


--
-- Name: departamento departamento_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departamento
    ADD CONSTRAINT departamento_pkey PRIMARY KEY (id);


--
-- Name: departamento_validador departamento_validador_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departamento_validador
    ADD CONSTRAINT departamento_validador_pkey PRIMARY KEY (departamento_id, usuario_id);


--
-- Name: documento_cxp_aprobacion documento_cxp_aprobacion_documento_id_usuario_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp_aprobacion
    ADD CONSTRAINT documento_cxp_aprobacion_documento_id_usuario_id_key UNIQUE (documento_id, usuario_id);


--
-- Name: documento_cxp_aprobacion documento_cxp_aprobacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp_aprobacion
    ADD CONSTRAINT documento_cxp_aprobacion_pkey PRIMARY KEY (id);


--
-- Name: documento_cxp documento_cxp_empresa_id_clave_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_empresa_id_clave_key UNIQUE (empresa_id, clave);


--
-- Name: documento_cxp documento_cxp_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_pkey PRIMARY KEY (id);


--
-- Name: empleado empleado_empresa_id_identificacion_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.empleado
    ADD CONSTRAINT empleado_empresa_id_identificacion_key UNIQUE (empresa_id, identificacion);


--
-- Name: empleado empleado_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.empleado
    ADD CONSTRAINT empleado_pkey PRIMARY KEY (id);


--
-- Name: empresa empresa_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.empresa
    ADD CONSTRAINT empresa_nombre_key UNIQUE (nombre);


--
-- Name: empresa empresa_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.empresa
    ADD CONSTRAINT empresa_pkey PRIMARY KEY (id);


--
-- Name: finiquito finiquito_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.finiquito
    ADD CONSTRAINT finiquito_pkey PRIMARY KEY (id);


--
-- Name: gestion_cxc gestion_cxc_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gestion_cxc
    ADD CONSTRAINT gestion_cxc_pkey PRIMARY KEY (id);


--
-- Name: importacion importacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.importacion
    ADD CONSTRAINT importacion_pkey PRIMARY KEY (id);


--
-- Name: incapacidad incapacidad_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incapacidad
    ADD CONSTRAINT incapacidad_pkey PRIMARY KEY (id);


--
-- Name: lote_pago lote_pago_empresa_id_numero_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lote_pago
    ADD CONSTRAINT lote_pago_empresa_id_numero_key UNIQUE (empresa_id, numero);


--
-- Name: lote_pago lote_pago_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lote_pago
    ADD CONSTRAINT lote_pago_pkey PRIMARY KEY (id);


--
-- Name: movimiento_bancario movimiento_bancario_empresa_id_natural_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.movimiento_bancario
    ADD CONSTRAINT movimiento_bancario_empresa_id_natural_key_key UNIQUE (empresa_id, natural_key);


--
-- Name: movimiento_bancario movimiento_bancario_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.movimiento_bancario
    ADD CONSTRAINT movimiento_bancario_pkey PRIMARY KEY (id);


--
-- Name: nomina_archivo_pago nomina_archivo_pago_empresa_id_consecutivo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nomina_archivo_pago
    ADD CONSTRAINT nomina_archivo_pago_empresa_id_consecutivo_key UNIQUE (empresa_id, consecutivo);


--
-- Name: nomina_archivo_pago nomina_archivo_pago_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nomina_archivo_pago
    ADD CONSTRAINT nomina_archivo_pago_pkey PRIMARY KEY (id);


--
-- Name: nomina_parametros nomina_parametros_empresa_id_anio_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nomina_parametros
    ADD CONSTRAINT nomina_parametros_empresa_id_anio_key UNIQUE (empresa_id, anio);


--
-- Name: nomina_parametros nomina_parametros_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nomina_parametros
    ADD CONSTRAINT nomina_parametros_pkey PRIMARY KEY (id);


--
-- Name: nota_credito_aplicacion nota_credito_aplicacion_nota_id_cargo_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_aplicacion
    ADD CONSTRAINT nota_credito_aplicacion_nota_id_cargo_id_key UNIQUE (nota_id, cargo_id);


--
-- Name: nota_credito_aplicacion nota_credito_aplicacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_aplicacion
    ADD CONSTRAINT nota_credito_aplicacion_pkey PRIMARY KEY (id);


--
-- Name: nota_credito_cxc nota_credito_consecutivo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_cxc
    ADD CONSTRAINT nota_credito_consecutivo_key UNIQUE (empresa_id, consecutivo);


--
-- Name: nota_credito_cxc nota_credito_cxc_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_cxc
    ADD CONSTRAINT nota_credito_cxc_pkey PRIMARY KEY (id);


--
-- Name: palabra_clave palabra_clave_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.palabra_clave
    ADD CONSTRAINT palabra_clave_pkey PRIMARY KEY (id);


--
-- Name: partida_conciliacion partida_conciliacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.partida_conciliacion
    ADD CONSTRAINT partida_conciliacion_pkey PRIMARY KEY (id);


--
-- Name: periodo_cierre periodo_cierre_empresa_id_anio_mes_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.periodo_cierre
    ADD CONSTRAINT periodo_cierre_empresa_id_anio_mes_key UNIQUE (empresa_id, anio, mes);


--
-- Name: periodo_cierre periodo_cierre_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.periodo_cierre
    ADD CONSTRAINT periodo_cierre_pkey PRIMARY KEY (id);


--
-- Name: permiso permiso_codigo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permiso
    ADD CONSTRAINT permiso_codigo_key UNIQUE (codigo);


--
-- Name: permiso permiso_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permiso
    ADD CONSTRAINT permiso_pkey PRIMARY KEY (id);


--
-- Name: plantilla_correo plantilla_correo_empresa_id_clave_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plantilla_correo
    ADD CONSTRAINT plantilla_correo_empresa_id_clave_key UNIQUE (empresa_id, clave);


--
-- Name: plantilla_correo plantilla_correo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plantilla_correo
    ADD CONSTRAINT plantilla_correo_pkey PRIMARY KEY (id);


--
-- Name: promesa_pago_cxc promesa_pago_cxc_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promesa_pago_cxc
    ADD CONSTRAINT promesa_pago_cxc_pkey PRIMARY KEY (id);


--
-- Name: proveedor proveedor_empresa_id_identificacion_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor
    ADD CONSTRAINT proveedor_empresa_id_identificacion_key UNIQUE (empresa_id, identificacion);


--
-- Name: proveedor_gasto proveedor_gasto_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor_gasto
    ADD CONSTRAINT proveedor_gasto_pkey PRIMARY KEY (id);


--
-- Name: proveedor_gasto proveedor_gasto_proveedor_id_concepto_id_clasificacion_id_s_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor_gasto
    ADD CONSTRAINT proveedor_gasto_proveedor_id_concepto_id_clasificacion_id_s_key UNIQUE (proveedor_id, concepto_id, clasificacion_id, subclasificacion_id);


--
-- Name: proveedor proveedor_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor
    ADD CONSTRAINT proveedor_pkey PRIMARY KEY (id);


--
-- Name: proyeccion_escenario proyeccion_escenario_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proyeccion_escenario
    ADD CONSTRAINT proyeccion_escenario_pkey PRIMARY KEY (id);


--
-- Name: regla_clasificacion regla_clasificacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.regla_clasificacion
    ADD CONSTRAINT regla_clasificacion_pkey PRIMARY KEY (id);


--
-- Name: rol rol_codigo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol
    ADD CONSTRAINT rol_codigo_key UNIQUE (codigo);


--
-- Name: rol_permiso rol_permiso_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol_permiso
    ADD CONSTRAINT rol_permiso_pkey PRIMARY KEY (empresa_id, rol_id, permiso_id);


--
-- Name: rol rol_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol
    ADD CONSTRAINT rol_pkey PRIMARY KEY (id);


--
-- Name: saldo_cuenta_diario saldo_cuenta_diario_empresa_id_cuenta_bancaria_id_fecha_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saldo_cuenta_diario
    ADD CONSTRAINT saldo_cuenta_diario_empresa_id_cuenta_bancaria_id_fecha_key UNIQUE (empresa_id, cuenta_bancaria_id, fecha);


--
-- Name: saldo_cuenta_diario saldo_cuenta_diario_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saldo_cuenta_diario
    ADD CONSTRAINT saldo_cuenta_diario_pkey PRIMARY KEY (id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: sesion sesion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sesion
    ADD CONSTRAINT sesion_pkey PRIMARY KEY (id);


--
-- Name: sesion sesion_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sesion
    ADD CONSTRAINT sesion_token_hash_key UNIQUE (token_hash);


--
-- Name: subclasificacion subclasificacion_clasificacion_id_nombre_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subclasificacion
    ADD CONSTRAINT subclasificacion_clasificacion_id_nombre_key UNIQUE (clasificacion_id, nombre);


--
-- Name: subclasificacion subclasificacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subclasificacion
    ADD CONSTRAINT subclasificacion_pkey PRIMARY KEY (id);


--
-- Name: tipo_cambio_cotizacion tipo_cambio_cotizacion_empresa_id_fecha_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tipo_cambio_cotizacion
    ADD CONSTRAINT tipo_cambio_cotizacion_empresa_id_fecha_key UNIQUE (empresa_id, fecha);


--
-- Name: tipo_cambio_cotizacion tipo_cambio_cotizacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tipo_cambio_cotizacion
    ADD CONSTRAINT tipo_cambio_cotizacion_pkey PRIMARY KEY (id);


--
-- Name: tipo_cambio_mes tipo_cambio_mes_empresa_id_anio_mes_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tipo_cambio_mes
    ADD CONSTRAINT tipo_cambio_mes_empresa_id_anio_mes_key UNIQUE (empresa_id, anio, mes);


--
-- Name: tipo_cambio_mes tipo_cambio_mes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tipo_cambio_mes
    ADD CONSTRAINT tipo_cambio_mes_pkey PRIMARY KEY (id);


--
-- Name: nomina_archivo_pago uniq_archivo_corrida; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nomina_archivo_pago
    ADD CONSTRAINT uniq_archivo_corrida UNIQUE (empresa_id, corrida_id);


--
-- Name: banco uq_banco_empresa_nombre; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.banco
    ADD CONSTRAINT uq_banco_empresa_nombre UNIQUE (empresa_id, nombre);


--
-- Name: cuenta_bancaria uq_cuenta_empresa_alias; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_bancaria
    ADD CONSTRAINT uq_cuenta_empresa_alias UNIQUE (empresa_id, alias);


--
-- Name: usuario usuario_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usuario
    ADD CONSTRAINT usuario_email_key UNIQUE (email);


--
-- Name: usuario_empresa_rol usuario_empresa_rol_empresa_id_usuario_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usuario_empresa_rol
    ADD CONSTRAINT usuario_empresa_rol_empresa_id_usuario_id_key UNIQUE (empresa_id, usuario_id);


--
-- Name: usuario_empresa_rol usuario_empresa_rol_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usuario_empresa_rol
    ADD CONSTRAINT usuario_empresa_rol_pkey PRIMARY KEY (id);


--
-- Name: usuario usuario_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usuario
    ADD CONSTRAINT usuario_pkey PRIMARY KEY (id);


--
-- Name: vacacion vacacion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vacacion
    ADD CONSTRAINT vacacion_pkey PRIMARY KEY (id);


--
-- Name: idx_anticipo_aplicacion_anticipo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_anticipo_aplicacion_anticipo ON public.anticipo_aplicacion USING btree (anticipo_id) WHERE activo;


--
-- Name: idx_anticipo_aplicacion_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_anticipo_aplicacion_empresa ON public.anticipo_aplicacion USING btree (empresa_id);


--
-- Name: idx_anticipo_aplicacion_factura; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_anticipo_aplicacion_factura ON public.anticipo_aplicacion USING btree (factura_id) WHERE activo;


--
-- Name: idx_arreglo_cuota_arreglo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_arreglo_cuota_arreglo ON public.arreglo_cuota_cxc USING btree (arreglo_id, vence_en);


--
-- Name: idx_arreglo_cxc_contrato; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_arreglo_cxc_contrato ON public.arreglo_pago_cxc USING btree (contrato_id, creado_en DESC);


--
-- Name: idx_arreglo_cxc_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_arreglo_cxc_empresa ON public.arreglo_pago_cxc USING btree (empresa_id, creado_en DESC);


--
-- Name: idx_arreglo_cxc_vivo; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_arreglo_cxc_vivo ON public.arreglo_pago_cxc USING btree (contrato_id) WHERE ((quebrado_en IS NULL) AND (anulado_en IS NULL));


--
-- Name: idx_auditoria_empresa_ts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auditoria_empresa_ts ON public.auditoria_evento USING btree (empresa_id, ts);


--
-- Name: idx_auditoria_entidad; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auditoria_entidad ON public.auditoria_evento USING btree (entidad, entidad_id);


--
-- Name: idx_banco_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_banco_empresa ON public.banco USING btree (empresa_id);


--
-- Name: idx_bccr_sync_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bccr_sync_empresa ON public.bccr_sync_log USING btree (empresa_id, creado_en DESC);


--
-- Name: idx_caja_fondo_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_caja_fondo_empresa ON public.caja_chica_fondo USING btree (empresa_id);


--
-- Name: idx_caja_vale_fondo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_caja_vale_fondo ON public.caja_chica_vale USING btree (fondo_id) WHERE (NOT anulado);


--
-- Name: idx_caja_vale_reposicion; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_caja_vale_reposicion ON public.caja_chica_vale USING btree (reposicion_id) WHERE (reposicion_id IS NOT NULL);


--
-- Name: idx_cargo_cxc_abierto; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cargo_cxc_abierto ON public.cargo_cxc USING btree (empresa_id, vence_en) WHERE (estado = ANY (ARRAY['ABIERTO'::text, 'PARCIAL'::text]));


--
-- Name: idx_cargo_cxc_contrato; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cargo_cxc_contrato ON public.cargo_cxc USING btree (contrato_id, vence_en);


--
-- Name: idx_cargo_cxc_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cargo_cxc_empresa ON public.cargo_cxc USING btree (empresa_id);


--
-- Name: idx_cargo_cxc_saldo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cargo_cxc_saldo ON public.cargo_cxc USING btree (empresa_id, contrato_id) INCLUDE (vence_en, monto, monto_aplicado) WHERE (estado = ANY (ARRAY['ABIERTO'::text, 'PARCIAL'::text]));


--
-- Name: idx_clasificacion_concepto; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_clasificacion_concepto ON public.clasificacion USING btree (concepto_id);


--
-- Name: idx_clasificacion_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_clasificacion_empresa ON public.clasificacion USING btree (empresa_id);


--
-- Name: idx_cobro_aplicacion_cargo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cobro_aplicacion_cargo ON public.cobro_aplicacion USING btree (cargo_id);


--
-- Name: idx_cobro_aplicacion_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cobro_aplicacion_empresa ON public.cobro_aplicacion USING btree (empresa_id);


--
-- Name: idx_cobro_cxc_consecutivo; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_cobro_cxc_consecutivo ON public.cobro_cxc USING btree (empresa_id, contrato_id, consecutivo) WHERE ((contrato_id IS NOT NULL) AND (consecutivo <> ''::text));


--
-- Name: idx_cobro_cxc_contrato; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cobro_cxc_contrato ON public.cobro_cxc USING btree (contrato_id, fecha_pago DESC);


--
-- Name: idx_cobro_cxc_contrato_origen; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cobro_cxc_contrato_origen ON public.cobro_cxc USING btree (empresa_id, contrato_origen) WHERE ((contrato_origen <> ''::text) AND (contrato_id IS NULL));


--
-- Name: idx_cobro_cxc_contrato_vivo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cobro_cxc_contrato_vivo ON public.cobro_cxc USING btree (contrato_id, fecha_bancaria) WHERE (estado <> 'REVERSADO'::text);


--
-- Name: idx_cobro_cxc_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cobro_cxc_empresa ON public.cobro_cxc USING btree (empresa_id, fecha_pago DESC);


--
-- Name: idx_cobro_cxc_idempotencia; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_cobro_cxc_idempotencia ON public.cobro_cxc USING btree (empresa_id, idempotency_key) WHERE ((idempotency_key IS NOT NULL) AND (idempotency_key <> ''::text));


--
-- Name: idx_cobro_cxc_planilla; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cobro_cxc_planilla ON public.cobro_cxc USING btree (planilla_id);


--
-- Name: idx_cobro_cxc_sin_identificar; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cobro_cxc_sin_identificar ON public.cobro_cxc USING btree (empresa_id, fecha_bancaria) WHERE (estado = 'SIN_IDENTIFICAR'::text);


--
-- Name: idx_comprobante_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_comprobante_empresa ON public.comprobante_pago USING btree (empresa_id);


--
-- Name: idx_concepto_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_concepto_empresa ON public.concepto USING btree (empresa_id);


--
-- Name: idx_concepto_naturaleza; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_concepto_naturaleza ON public.concepto USING btree (empresa_id, naturaleza);


--
-- Name: idx_contrato_cxc_asociacion; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contrato_cxc_asociacion ON public.contrato_cxc USING btree (empresa_id, asociacion_id);


--
-- Name: idx_contrato_cxc_documento; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contrato_cxc_documento ON public.contrato_cxc USING btree (empresa_id, documento);


--
-- Name: idx_contrato_cxc_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contrato_cxc_empresa ON public.contrato_cxc USING btree (empresa_id);


--
-- Name: idx_contrato_cxc_nombre_trgm; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contrato_cxc_nombre_trgm ON public.contrato_cxc USING gin (cliente_nombre public.gin_trgm_ops);


--
-- Name: idx_contrato_cxc_sede; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contrato_cxc_sede ON public.contrato_cxc USING btree (empresa_id, sede_id);


--
-- Name: idx_corrida_linea; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_corrida_linea ON public.corrida_linea USING btree (corrida_id);


--
-- Name: idx_corrida_novedad; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_corrida_novedad ON public.corrida_novedad USING btree (corrida_id);


--
-- Name: idx_cuenta_banco; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cuenta_banco ON public.cuenta_bancaria USING btree (banco_id);


--
-- Name: idx_cuenta_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cuenta_empresa ON public.cuenta_bancaria USING btree (empresa_id);


--
-- Name: idx_cxc_asociacion_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cxc_asociacion_empresa ON public.cxc_asociacion USING btree (empresa_id);


--
-- Name: idx_cxc_forma_pago_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cxc_forma_pago_empresa ON public.cxc_forma_pago USING btree (empresa_id);


--
-- Name: idx_cxc_importacion_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cxc_importacion_empresa ON public.cxc_importacion USING btree (empresa_id, creado_en DESC);


--
-- Name: idx_cxc_modalidad_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cxc_modalidad_empresa ON public.cxc_modalidad USING btree (empresa_id);


--
-- Name: idx_cxc_planilla_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cxc_planilla_empresa ON public.cxc_planilla USING btree (empresa_id, periodo);


--
-- Name: idx_cxc_sede_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cxc_sede_empresa ON public.cxc_sede USING btree (empresa_id);


--
-- Name: idx_cxc_suspension_contrato; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cxc_suspension_contrato ON public.cxc_suspension USING btree (contrato_id, suspendido_en DESC);


--
-- Name: idx_cxc_suspension_vigente; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_cxc_suspension_vigente ON public.cxc_suspension USING btree (contrato_id) WHERE (reactivado_en IS NULL);


--
-- Name: idx_cxc_usuario_sede_usuario; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cxc_usuario_sede_usuario ON public.cxc_usuario_sede USING btree (empresa_id, usuario_id);


--
-- Name: idx_deduccion_empleado; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_deduccion_empleado ON public.deduccion_empleado USING btree (empleado_id) WHERE activo;


--
-- Name: idx_departamento_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_departamento_empresa ON public.departamento USING btree (empresa_id);


--
-- Name: idx_documento_departamento; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_documento_departamento ON public.documento_cxp USING btree (departamento_id);


--
-- Name: idx_docxp_aprob_doc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_docxp_aprob_doc ON public.documento_cxp_aprobacion USING btree (documento_id);


--
-- Name: idx_docxp_concepto; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_docxp_concepto ON public.documento_cxp USING btree (concepto_id);


--
-- Name: idx_docxp_contabilidad; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_docxp_contabilidad ON public.documento_cxp USING btree (empresa_id, estado) WHERE (es_contabilidad IS TRUE);


--
-- Name: idx_docxp_empresa_estado; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_docxp_empresa_estado ON public.documento_cxp USING btree (empresa_id, estado);


--
-- Name: idx_docxp_lote; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_docxp_lote ON public.documento_cxp USING btree (lote_id);


--
-- Name: idx_docxp_proveedor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_docxp_proveedor ON public.documento_cxp USING btree (proveedor_id);


--
-- Name: idx_docxp_requiere_validacion; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_docxp_requiere_validacion ON public.documento_cxp USING btree (empresa_id, estado) WHERE (requiere_validacion IS TRUE);


--
-- Name: idx_docxp_subclasif; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_docxp_subclasif ON public.documento_cxp USING btree (subclasificacion_id);


--
-- Name: idx_docxp_tipo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_docxp_tipo ON public.documento_cxp USING btree (empresa_id, tipo);


--
-- Name: idx_docxp_vencimiento; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_docxp_vencimiento ON public.documento_cxp USING btree (empresa_id, fecha_vencimiento);


--
-- Name: idx_empleado_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_empleado_empresa ON public.empleado USING btree (empresa_id) WHERE activo;


--
-- Name: idx_gestion_cxc_contrato; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gestion_cxc_contrato ON public.gestion_cxc USING btree (contrato_id, gestionada_en DESC);


--
-- Name: idx_gestion_cxc_fecha; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gestion_cxc_fecha ON public.gestion_cxc USING btree (empresa_id, gestionada_en DESC);


--
-- Name: idx_gestion_cxc_usuario; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_gestion_cxc_usuario ON public.gestion_cxc USING btree (empresa_id, usuario_id, gestionada_en DESC);


--
-- Name: idx_importacion_cuenta; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_importacion_cuenta ON public.importacion USING btree (cuenta_bancaria_id);


--
-- Name: idx_importacion_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_importacion_empresa ON public.importacion USING btree (empresa_id);


--
-- Name: idx_incapacidad_empleado; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_incapacidad_empleado ON public.incapacidad USING btree (empresa_id, empleado_id) WHERE (NOT anulada);


--
-- Name: idx_incapacidad_fecha; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_incapacidad_fecha ON public.incapacidad USING btree (empresa_id, fecha_inicio) WHERE (NOT anulada);


--
-- Name: idx_lote_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lote_empresa ON public.lote_pago USING btree (empresa_id, creado_en DESC);


--
-- Name: idx_mov_cuenta; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mov_cuenta ON public.movimiento_bancario USING btree (cuenta_bancaria_id);


--
-- Name: idx_mov_documento_cxp; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mov_documento_cxp ON public.movimiento_bancario USING btree (documento_cxp_id) WHERE (documento_cxp_id IS NOT NULL);


--
-- Name: idx_mov_empresa_estado; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mov_empresa_estado ON public.movimiento_bancario USING btree (empresa_id, estado_clasificacion);


--
-- Name: idx_mov_empresa_fecha; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mov_empresa_fecha ON public.movimiento_bancario USING btree (empresa_id, fecha);


--
-- Name: idx_mov_huella_pendiente; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mov_huella_pendiente ON public.movimiento_bancario USING btree (empresa_id) WHERE (documento_cxp_id IS NULL);


--
-- Name: idx_mov_importacion; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mov_importacion ON public.movimiento_bancario USING btree (importacion_id);


--
-- Name: idx_nota_credito_aplic_cargo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_nota_credito_aplic_cargo ON public.nota_credito_aplicacion USING btree (cargo_id);


--
-- Name: idx_nota_credito_aplic_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_nota_credito_aplic_empresa ON public.nota_credito_aplicacion USING btree (empresa_id);


--
-- Name: idx_nota_credito_cxc_contrato; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_nota_credito_cxc_contrato ON public.nota_credito_cxc USING btree (contrato_id, fecha DESC);


--
-- Name: idx_nota_credito_cxc_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_nota_credito_cxc_empresa ON public.nota_credito_cxc USING btree (empresa_id, fecha DESC);


--
-- Name: idx_palabra_regla; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_palabra_regla ON public.palabra_clave USING btree (regla_id);


--
-- Name: idx_partida_conc_cuenta_mes; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_partida_conc_cuenta_mes ON public.partida_conciliacion USING btree (empresa_id, cuenta_bancaria_id, anio, mes) WHERE (NOT anulada);


--
-- Name: idx_periodo_cierre_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_periodo_cierre_empresa ON public.periodo_cierre USING btree (empresa_id, anio, mes);


--
-- Name: idx_planilla_mov_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_planilla_mov_empresa ON public.cxc_planilla_movimiento USING btree (empresa_id);


--
-- Name: idx_promesa_cxc_contrato; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_promesa_cxc_contrato ON public.promesa_pago_cxc USING btree (contrato_id, fecha_promesa DESC);


--
-- Name: idx_promesa_cxc_fecha; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_promesa_cxc_fecha ON public.promesa_pago_cxc USING btree (empresa_id, fecha_promesa DESC);


--
-- Name: idx_proveedor_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_proveedor_empresa ON public.proveedor USING btree (empresa_id);


--
-- Name: idx_provgasto_prov; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_provgasto_prov ON public.proveedor_gasto USING btree (proveedor_id, usos DESC);


--
-- Name: idx_proyeccion_empresa_periodo; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_proyeccion_empresa_periodo ON public.proyeccion_escenario USING btree (empresa_id, periodo);


--
-- Name: idx_regla_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_regla_empresa ON public.regla_clasificacion USING btree (empresa_id);


--
-- Name: idx_rolpermiso_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rolpermiso_lookup ON public.rol_permiso USING btree (empresa_id, rol_id);


--
-- Name: idx_saldo_diario_cuenta; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_saldo_diario_cuenta ON public.saldo_cuenta_diario USING btree (empresa_id, cuenta_bancaria_id, fecha DESC);


--
-- Name: idx_saldo_diario_fecha; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_saldo_diario_fecha ON public.saldo_cuenta_diario USING btree (empresa_id, fecha);


--
-- Name: idx_sesion_usuario; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sesion_usuario ON public.sesion USING btree (usuario_id);


--
-- Name: idx_subclasif_clasif; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subclasif_clasif ON public.subclasificacion USING btree (clasificacion_id);


--
-- Name: idx_subclasif_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subclasif_empresa ON public.subclasificacion USING btree (empresa_id);


--
-- Name: idx_tc_cot_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tc_cot_empresa ON public.tipo_cambio_cotizacion USING btree (empresa_id, fecha);


--
-- Name: idx_uer_empresa; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_uer_empresa ON public.usuario_empresa_rol USING btree (empresa_id);


--
-- Name: idx_uer_usuario; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_uer_usuario ON public.usuario_empresa_rol USING btree (usuario_id);


--
-- Name: idx_vacacion_empleado; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vacacion_empleado ON public.vacacion USING btree (empresa_id, empleado_id) WHERE (NOT anulada);


--
-- Name: uniq_corrida_viva; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uniq_corrida_viva ON public.corrida_nomina USING btree (empresa_id, anio, mes, tipo) WHERE (estado <> 'ANULADA'::text);


--
-- Name: uniq_finiquito_vivo; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uniq_finiquito_vivo ON public.finiquito USING btree (empresa_id, empleado_id) WHERE (estado <> 'ANULADO'::text);


--
-- Name: auditoria_evento auditoria_no_delete; Type: RULE; Schema: public; Owner: -
--

CREATE RULE auditoria_no_delete AS
    ON DELETE TO public.auditoria_evento DO INSTEAD NOTHING;


--
-- Name: auditoria_evento auditoria_no_update; Type: RULE; Schema: public; Owner: -
--

CREATE RULE auditoria_no_update AS
    ON UPDATE TO public.auditoria_evento DO INSTEAD NOTHING;


--
-- Name: auditoria_evento auditoria_no_truncate; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER auditoria_no_truncate BEFORE TRUNCATE ON public.auditoria_evento FOR EACH STATEMENT EXECUTE FUNCTION public.auditoria_no_truncate();


--
-- Name: acta_conciliacion acta_conciliacion_cuenta_bancaria_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acta_conciliacion
    ADD CONSTRAINT acta_conciliacion_cuenta_bancaria_id_fkey FOREIGN KEY (cuenta_bancaria_id) REFERENCES public.cuenta_bancaria(id);


--
-- Name: acta_conciliacion acta_conciliacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acta_conciliacion
    ADD CONSTRAINT acta_conciliacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: acta_conciliacion acta_conciliacion_firmado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acta_conciliacion
    ADD CONSTRAINT acta_conciliacion_firmado_por_fkey FOREIGN KEY (firmado_por) REFERENCES public.usuario(id);


--
-- Name: acta_conciliacion acta_conciliacion_preparado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acta_conciliacion
    ADD CONSTRAINT acta_conciliacion_preparado_por_fkey FOREIGN KEY (preparado_por) REFERENCES public.usuario(id);


--
-- Name: anticipo_aplicacion anticipo_aplicacion_anticipo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anticipo_aplicacion
    ADD CONSTRAINT anticipo_aplicacion_anticipo_id_fkey FOREIGN KEY (anticipo_id) REFERENCES public.documento_cxp(id);


--
-- Name: anticipo_aplicacion anticipo_aplicacion_aplicado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anticipo_aplicacion
    ADD CONSTRAINT anticipo_aplicacion_aplicado_por_fkey FOREIGN KEY (aplicado_por) REFERENCES public.usuario(id);


--
-- Name: anticipo_aplicacion anticipo_aplicacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anticipo_aplicacion
    ADD CONSTRAINT anticipo_aplicacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: anticipo_aplicacion anticipo_aplicacion_factura_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anticipo_aplicacion
    ADD CONSTRAINT anticipo_aplicacion_factura_id_fkey FOREIGN KEY (factura_id) REFERENCES public.documento_cxp(id);


--
-- Name: anticipo_aplicacion anticipo_aplicacion_reversado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anticipo_aplicacion
    ADD CONSTRAINT anticipo_aplicacion_reversado_por_fkey FOREIGN KEY (reversado_por) REFERENCES public.usuario(id);


--
-- Name: arreglo_cuota_cxc arreglo_cuota_cxc_arreglo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_cuota_cxc
    ADD CONSTRAINT arreglo_cuota_cxc_arreglo_id_fkey FOREIGN KEY (arreglo_id) REFERENCES public.arreglo_pago_cxc(id) ON DELETE CASCADE;


--
-- Name: arreglo_cuota_cxc arreglo_cuota_cxc_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_cuota_cxc
    ADD CONSTRAINT arreglo_cuota_cxc_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: arreglo_pago_cxc arreglo_pago_cxc_anulado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_pago_cxc
    ADD CONSTRAINT arreglo_pago_cxc_anulado_por_fkey FOREIGN KEY (anulado_por) REFERENCES public.usuario(id);


--
-- Name: arreglo_pago_cxc arreglo_pago_cxc_autorizado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_pago_cxc
    ADD CONSTRAINT arreglo_pago_cxc_autorizado_por_fkey FOREIGN KEY (autorizado_por) REFERENCES public.usuario(id);


--
-- Name: arreglo_pago_cxc arreglo_pago_cxc_contrato_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_pago_cxc
    ADD CONSTRAINT arreglo_pago_cxc_contrato_id_fkey FOREIGN KEY (contrato_id) REFERENCES public.contrato_cxc(id);


--
-- Name: arreglo_pago_cxc arreglo_pago_cxc_creado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_pago_cxc
    ADD CONSTRAINT arreglo_pago_cxc_creado_por_fkey FOREIGN KEY (creado_por) REFERENCES public.usuario(id);


--
-- Name: arreglo_pago_cxc arreglo_pago_cxc_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_pago_cxc
    ADD CONSTRAINT arreglo_pago_cxc_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: arreglo_pago_cxc arreglo_pago_cxc_quebrado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.arreglo_pago_cxc
    ADD CONSTRAINT arreglo_pago_cxc_quebrado_por_fkey FOREIGN KEY (quebrado_por) REFERENCES public.usuario(id);


--
-- Name: auditoria_evento auditoria_evento_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auditoria_evento
    ADD CONSTRAINT auditoria_evento_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: auditoria_evento auditoria_evento_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auditoria_evento
    ADD CONSTRAINT auditoria_evento_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id);


--
-- Name: banco banco_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.banco
    ADD CONSTRAINT banco_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: bccr_sync_log bccr_sync_log_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bccr_sync_log
    ADD CONSTRAINT bccr_sync_log_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: caja_chica_fondo caja_chica_fondo_custodio_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_fondo
    ADD CONSTRAINT caja_chica_fondo_custodio_id_fkey FOREIGN KEY (custodio_id) REFERENCES public.usuario(id);


--
-- Name: caja_chica_fondo caja_chica_fondo_departamento_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_fondo
    ADD CONSTRAINT caja_chica_fondo_departamento_id_fkey FOREIGN KEY (departamento_id) REFERENCES public.departamento(id);


--
-- Name: caja_chica_fondo caja_chica_fondo_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_fondo
    ADD CONSTRAINT caja_chica_fondo_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: caja_chica_fondo caja_chica_fondo_proveedor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_fondo
    ADD CONSTRAINT caja_chica_fondo_proveedor_id_fkey FOREIGN KEY (proveedor_id) REFERENCES public.proveedor(id);


--
-- Name: caja_chica_vale caja_chica_vale_clasificacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_vale
    ADD CONSTRAINT caja_chica_vale_clasificacion_id_fkey FOREIGN KEY (clasificacion_id) REFERENCES public.clasificacion(id);


--
-- Name: caja_chica_vale caja_chica_vale_concepto_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_vale
    ADD CONSTRAINT caja_chica_vale_concepto_id_fkey FOREIGN KEY (concepto_id) REFERENCES public.concepto(id);


--
-- Name: caja_chica_vale caja_chica_vale_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_vale
    ADD CONSTRAINT caja_chica_vale_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: caja_chica_vale caja_chica_vale_fondo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_vale
    ADD CONSTRAINT caja_chica_vale_fondo_id_fkey FOREIGN KEY (fondo_id) REFERENCES public.caja_chica_fondo(id);


--
-- Name: caja_chica_vale caja_chica_vale_registrado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_vale
    ADD CONSTRAINT caja_chica_vale_registrado_por_fkey FOREIGN KEY (registrado_por) REFERENCES public.usuario(id);


--
-- Name: caja_chica_vale caja_chica_vale_reposicion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_vale
    ADD CONSTRAINT caja_chica_vale_reposicion_id_fkey FOREIGN KEY (reposicion_id) REFERENCES public.documento_cxp(id);


--
-- Name: caja_chica_vale caja_chica_vale_subclasificacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.caja_chica_vale
    ADD CONSTRAINT caja_chica_vale_subclasificacion_id_fkey FOREIGN KEY (subclasificacion_id) REFERENCES public.subclasificacion(id);


--
-- Name: cargo_cxc cargo_cxc_contrato_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cargo_cxc
    ADD CONSTRAINT cargo_cxc_contrato_id_fkey FOREIGN KEY (contrato_id) REFERENCES public.contrato_cxc(id) ON DELETE CASCADE;


--
-- Name: cargo_cxc cargo_cxc_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cargo_cxc
    ADD CONSTRAINT cargo_cxc_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: clasificacion clasificacion_concepto_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.clasificacion
    ADD CONSTRAINT clasificacion_concepto_id_fkey FOREIGN KEY (concepto_id) REFERENCES public.concepto(id) ON DELETE CASCADE;


--
-- Name: clasificacion clasificacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.clasificacion
    ADD CONSTRAINT clasificacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cobro_aplicacion cobro_aplicacion_cargo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_aplicacion
    ADD CONSTRAINT cobro_aplicacion_cargo_id_fkey FOREIGN KEY (cargo_id) REFERENCES public.cargo_cxc(id) ON DELETE CASCADE;


--
-- Name: cobro_aplicacion cobro_aplicacion_cobro_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_aplicacion
    ADD CONSTRAINT cobro_aplicacion_cobro_id_fkey FOREIGN KEY (cobro_id) REFERENCES public.cobro_cxc(id) ON DELETE CASCADE;


--
-- Name: cobro_aplicacion cobro_aplicacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_aplicacion
    ADD CONSTRAINT cobro_aplicacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cobro_cxc cobro_cxc_asociacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_cxc
    ADD CONSTRAINT cobro_cxc_asociacion_id_fkey FOREIGN KEY (asociacion_id) REFERENCES public.cxc_asociacion(id);


--
-- Name: cobro_cxc cobro_cxc_contrato_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_cxc
    ADD CONSTRAINT cobro_cxc_contrato_id_fkey FOREIGN KEY (contrato_id) REFERENCES public.contrato_cxc(id);


--
-- Name: cobro_cxc cobro_cxc_creado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_cxc
    ADD CONSTRAINT cobro_cxc_creado_por_fkey FOREIGN KEY (creado_por) REFERENCES public.usuario(id);


--
-- Name: cobro_cxc cobro_cxc_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_cxc
    ADD CONSTRAINT cobro_cxc_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cobro_cxc cobro_cxc_forma_pago_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_cxc
    ADD CONSTRAINT cobro_cxc_forma_pago_id_fkey FOREIGN KEY (forma_pago_id) REFERENCES public.cxc_forma_pago(id);


--
-- Name: cobro_cxc cobro_cxc_movimiento_bancario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_cxc
    ADD CONSTRAINT cobro_cxc_movimiento_bancario_id_fkey FOREIGN KEY (movimiento_bancario_id) REFERENCES public.movimiento_bancario(id);


--
-- Name: cobro_cxc cobro_cxc_planilla_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_cxc
    ADD CONSTRAINT cobro_cxc_planilla_id_fkey FOREIGN KEY (planilla_id) REFERENCES public.cxc_planilla(id);


--
-- Name: cobro_cxc cobro_cxc_reversado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cobro_cxc
    ADD CONSTRAINT cobro_cxc_reversado_por_fkey FOREIGN KEY (reversado_por) REFERENCES public.usuario(id);


--
-- Name: comprobante_pago comprobante_pago_documento_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comprobante_pago
    ADD CONSTRAINT comprobante_pago_documento_id_fkey FOREIGN KEY (documento_id) REFERENCES public.documento_cxp(id) ON DELETE CASCADE;


--
-- Name: comprobante_pago comprobante_pago_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comprobante_pago
    ADD CONSTRAINT comprobante_pago_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: comprobante_pago comprobante_pago_subido_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comprobante_pago
    ADD CONSTRAINT comprobante_pago_subido_por_fkey FOREIGN KEY (subido_por) REFERENCES public.usuario(id);


--
-- Name: concepto concepto_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.concepto
    ADD CONSTRAINT concepto_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: concepto_nomina concepto_nomina_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.concepto_nomina
    ADD CONSTRAINT concepto_nomina_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: contrato_cxc contrato_cxc_asociacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_cxc
    ADD CONSTRAINT contrato_cxc_asociacion_id_fkey FOREIGN KEY (asociacion_id) REFERENCES public.cxc_asociacion(id);


--
-- Name: contrato_cxc contrato_cxc_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_cxc
    ADD CONSTRAINT contrato_cxc_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: contrato_cxc contrato_cxc_forma_pago_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_cxc
    ADD CONSTRAINT contrato_cxc_forma_pago_id_fkey FOREIGN KEY (forma_pago_id) REFERENCES public.cxc_forma_pago(id);


--
-- Name: contrato_cxc contrato_cxc_modalidad_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_cxc
    ADD CONSTRAINT contrato_cxc_modalidad_id_fkey FOREIGN KEY (modalidad_id) REFERENCES public.cxc_modalidad(id);


--
-- Name: contrato_cxc contrato_cxc_sede_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contrato_cxc
    ADD CONSTRAINT contrato_cxc_sede_id_fkey FOREIGN KEY (sede_id) REFERENCES public.cxc_sede(id);


--
-- Name: corrida_linea corrida_linea_corrida_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_linea
    ADD CONSTRAINT corrida_linea_corrida_id_fkey FOREIGN KEY (corrida_id) REFERENCES public.corrida_nomina(id) ON DELETE CASCADE;


--
-- Name: corrida_linea corrida_linea_empleado_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_linea
    ADD CONSTRAINT corrida_linea_empleado_id_fkey FOREIGN KEY (empleado_id) REFERENCES public.empleado(id);


--
-- Name: corrida_linea corrida_linea_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_linea
    ADD CONSTRAINT corrida_linea_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: corrida_nomina corrida_nomina_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_nomina
    ADD CONSTRAINT corrida_nomina_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: corrida_novedad corrida_novedad_concepto_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_novedad
    ADD CONSTRAINT corrida_novedad_concepto_id_fkey FOREIGN KEY (concepto_id) REFERENCES public.concepto_nomina(id);


--
-- Name: corrida_novedad corrida_novedad_corrida_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_novedad
    ADD CONSTRAINT corrida_novedad_corrida_id_fkey FOREIGN KEY (corrida_id) REFERENCES public.corrida_nomina(id) ON DELETE CASCADE;


--
-- Name: corrida_novedad corrida_novedad_empleado_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_novedad
    ADD CONSTRAINT corrida_novedad_empleado_id_fkey FOREIGN KEY (empleado_id) REFERENCES public.empleado(id);


--
-- Name: corrida_novedad corrida_novedad_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corrida_novedad
    ADD CONSTRAINT corrida_novedad_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: cuenta_bancaria cuenta_bancaria_banco_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_bancaria
    ADD CONSTRAINT cuenta_bancaria_banco_id_fkey FOREIGN KEY (banco_id) REFERENCES public.banco(id);


--
-- Name: cuenta_bancaria cuenta_bancaria_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cuenta_bancaria
    ADD CONSTRAINT cuenta_bancaria_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_asociacion cxc_asociacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_asociacion
    ADD CONSTRAINT cxc_asociacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_canal_gestion cxc_canal_gestion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_canal_gestion
    ADD CONSTRAINT cxc_canal_gestion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_forma_pago cxc_forma_pago_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_forma_pago
    ADD CONSTRAINT cxc_forma_pago_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_importacion cxc_importacion_creado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_importacion
    ADD CONSTRAINT cxc_importacion_creado_por_fkey FOREIGN KEY (creado_por) REFERENCES public.usuario(id);


--
-- Name: cxc_importacion cxc_importacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_importacion
    ADD CONSTRAINT cxc_importacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_modalidad cxc_modalidad_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_modalidad
    ADD CONSTRAINT cxc_modalidad_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_parametro cxc_parametro_actualizado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_parametro
    ADD CONSTRAINT cxc_parametro_actualizado_por_fkey FOREIGN KEY (actualizado_por) REFERENCES public.usuario(id);


--
-- Name: cxc_parametro cxc_parametro_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_parametro
    ADD CONSTRAINT cxc_parametro_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_planilla cxc_planilla_asociacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla
    ADD CONSTRAINT cxc_planilla_asociacion_id_fkey FOREIGN KEY (asociacion_id) REFERENCES public.cxc_asociacion(id);


--
-- Name: cxc_planilla cxc_planilla_creado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla
    ADD CONSTRAINT cxc_planilla_creado_por_fkey FOREIGN KEY (creado_por) REFERENCES public.usuario(id);


--
-- Name: cxc_planilla cxc_planilla_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla
    ADD CONSTRAINT cxc_planilla_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_planilla_movimiento cxc_planilla_movimiento_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla_movimiento
    ADD CONSTRAINT cxc_planilla_movimiento_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_planilla_movimiento cxc_planilla_movimiento_movimiento_bancario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla_movimiento
    ADD CONSTRAINT cxc_planilla_movimiento_movimiento_bancario_id_fkey FOREIGN KEY (movimiento_bancario_id) REFERENCES public.movimiento_bancario(id) ON DELETE CASCADE;


--
-- Name: cxc_planilla_movimiento cxc_planilla_movimiento_planilla_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla_movimiento
    ADD CONSTRAINT cxc_planilla_movimiento_planilla_id_fkey FOREIGN KEY (planilla_id) REFERENCES public.cxc_planilla(id) ON DELETE CASCADE;


--
-- Name: cxc_planilla_movimiento cxc_planilla_movimiento_vinculado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_planilla_movimiento
    ADD CONSTRAINT cxc_planilla_movimiento_vinculado_por_fkey FOREIGN KEY (vinculado_por) REFERENCES public.usuario(id);


--
-- Name: cxc_resultado_gestion cxc_resultado_gestion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_resultado_gestion
    ADD CONSTRAINT cxc_resultado_gestion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_sede cxc_sede_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_sede
    ADD CONSTRAINT cxc_sede_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_suspension cxc_suspension_contrato_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_suspension
    ADD CONSTRAINT cxc_suspension_contrato_id_fkey FOREIGN KEY (contrato_id) REFERENCES public.contrato_cxc(id) ON DELETE CASCADE;


--
-- Name: cxc_suspension cxc_suspension_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_suspension
    ADD CONSTRAINT cxc_suspension_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_suspension cxc_suspension_reactivado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_suspension
    ADD CONSTRAINT cxc_suspension_reactivado_por_fkey FOREIGN KEY (reactivado_por) REFERENCES public.usuario(id);


--
-- Name: cxc_suspension cxc_suspension_suspendido_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_suspension
    ADD CONSTRAINT cxc_suspension_suspendido_por_fkey FOREIGN KEY (suspendido_por) REFERENCES public.usuario(id);


--
-- Name: cxc_tramo cxc_tramo_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_tramo
    ADD CONSTRAINT cxc_tramo_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_usuario_sede cxc_usuario_sede_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_usuario_sede
    ADD CONSTRAINT cxc_usuario_sede_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: cxc_usuario_sede cxc_usuario_sede_sede_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_usuario_sede
    ADD CONSTRAINT cxc_usuario_sede_sede_id_fkey FOREIGN KEY (sede_id) REFERENCES public.cxc_sede(id) ON DELETE CASCADE;


--
-- Name: cxc_usuario_sede cxc_usuario_sede_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxc_usuario_sede
    ADD CONSTRAINT cxc_usuario_sede_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: cxp_parametro cxp_parametro_actualizado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxp_parametro
    ADD CONSTRAINT cxp_parametro_actualizado_por_fkey FOREIGN KEY (actualizado_por) REFERENCES public.usuario(id);


--
-- Name: cxp_parametro cxp_parametro_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cxp_parametro
    ADD CONSTRAINT cxp_parametro_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: deduccion_empleado deduccion_empleado_concepto_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deduccion_empleado
    ADD CONSTRAINT deduccion_empleado_concepto_id_fkey FOREIGN KEY (concepto_id) REFERENCES public.concepto_nomina(id);


--
-- Name: deduccion_empleado deduccion_empleado_empleado_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deduccion_empleado
    ADD CONSTRAINT deduccion_empleado_empleado_id_fkey FOREIGN KEY (empleado_id) REFERENCES public.empleado(id);


--
-- Name: deduccion_empleado deduccion_empleado_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deduccion_empleado
    ADD CONSTRAINT deduccion_empleado_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: departamento departamento_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departamento
    ADD CONSTRAINT departamento_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: departamento_validador departamento_validador_departamento_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departamento_validador
    ADD CONSTRAINT departamento_validador_departamento_id_fkey FOREIGN KEY (departamento_id) REFERENCES public.departamento(id) ON DELETE CASCADE;


--
-- Name: departamento_validador departamento_validador_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departamento_validador
    ADD CONSTRAINT departamento_validador_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: documento_cxp_aprobacion documento_cxp_aprobacion_documento_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp_aprobacion
    ADD CONSTRAINT documento_cxp_aprobacion_documento_id_fkey FOREIGN KEY (documento_id) REFERENCES public.documento_cxp(id) ON DELETE CASCADE;


--
-- Name: documento_cxp_aprobacion documento_cxp_aprobacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp_aprobacion
    ADD CONSTRAINT documento_cxp_aprobacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: documento_cxp_aprobacion documento_cxp_aprobacion_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp_aprobacion
    ADD CONSTRAINT documento_cxp_aprobacion_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id);


--
-- Name: documento_cxp documento_cxp_clasificacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_clasificacion_id_fkey FOREIGN KEY (clasificacion_id) REFERENCES public.clasificacion(id);


--
-- Name: documento_cxp documento_cxp_concepto_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_concepto_id_fkey FOREIGN KEY (concepto_id) REFERENCES public.concepto(id);


--
-- Name: documento_cxp documento_cxp_contabilidad_marcado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_contabilidad_marcado_por_fkey FOREIGN KEY (contabilidad_marcado_por) REFERENCES public.usuario(id);


--
-- Name: documento_cxp documento_cxp_creado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_creado_por_fkey FOREIGN KEY (creado_por) REFERENCES public.usuario(id);


--
-- Name: documento_cxp documento_cxp_departamento_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_departamento_id_fkey FOREIGN KEY (departamento_id) REFERENCES public.departamento(id);


--
-- Name: documento_cxp documento_cxp_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: documento_cxp documento_cxp_lote_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_lote_id_fkey FOREIGN KEY (lote_id) REFERENCES public.lote_pago(id);


--
-- Name: documento_cxp documento_cxp_proveedor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_proveedor_id_fkey FOREIGN KEY (proveedor_id) REFERENCES public.proveedor(id);


--
-- Name: documento_cxp documento_cxp_subclasificacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.documento_cxp
    ADD CONSTRAINT documento_cxp_subclasificacion_id_fkey FOREIGN KEY (subclasificacion_id) REFERENCES public.subclasificacion(id);


--
-- Name: empleado empleado_departamento_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.empleado
    ADD CONSTRAINT empleado_departamento_id_fkey FOREIGN KEY (departamento_id) REFERENCES public.departamento(id);


--
-- Name: empleado empleado_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.empleado
    ADD CONSTRAINT empleado_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: finiquito finiquito_empleado_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.finiquito
    ADD CONSTRAINT finiquito_empleado_id_fkey FOREIGN KEY (empleado_id) REFERENCES public.empleado(id);


--
-- Name: finiquito finiquito_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.finiquito
    ADD CONSTRAINT finiquito_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: gestion_cxc gestion_cxc_canal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gestion_cxc
    ADD CONSTRAINT gestion_cxc_canal_id_fkey FOREIGN KEY (canal_id) REFERENCES public.cxc_canal_gestion(id);


--
-- Name: gestion_cxc gestion_cxc_contrato_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gestion_cxc
    ADD CONSTRAINT gestion_cxc_contrato_id_fkey FOREIGN KEY (contrato_id) REFERENCES public.contrato_cxc(id) ON DELETE CASCADE;


--
-- Name: gestion_cxc gestion_cxc_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gestion_cxc
    ADD CONSTRAINT gestion_cxc_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: gestion_cxc gestion_cxc_resultado_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gestion_cxc
    ADD CONSTRAINT gestion_cxc_resultado_id_fkey FOREIGN KEY (resultado_id) REFERENCES public.cxc_resultado_gestion(id);


--
-- Name: gestion_cxc gestion_cxc_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gestion_cxc
    ADD CONSTRAINT gestion_cxc_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id);


--
-- Name: importacion importacion_creado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.importacion
    ADD CONSTRAINT importacion_creado_por_fkey FOREIGN KEY (creado_por) REFERENCES public.usuario(id);


--
-- Name: importacion importacion_cuenta_bancaria_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.importacion
    ADD CONSTRAINT importacion_cuenta_bancaria_id_fkey FOREIGN KEY (cuenta_bancaria_id) REFERENCES public.cuenta_bancaria(id);


--
-- Name: importacion importacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.importacion
    ADD CONSTRAINT importacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: incapacidad incapacidad_empleado_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incapacidad
    ADD CONSTRAINT incapacidad_empleado_id_fkey FOREIGN KEY (empleado_id) REFERENCES public.empleado(id);


--
-- Name: incapacidad incapacidad_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incapacidad
    ADD CONSTRAINT incapacidad_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: lote_pago lote_pago_creado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lote_pago
    ADD CONSTRAINT lote_pago_creado_por_fkey FOREIGN KEY (creado_por) REFERENCES public.usuario(id);


--
-- Name: lote_pago lote_pago_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lote_pago
    ADD CONSTRAINT lote_pago_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: movimiento_bancario movimiento_bancario_clasificacion_id_concepto_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.movimiento_bancario
    ADD CONSTRAINT movimiento_bancario_clasificacion_id_concepto_id_fkey FOREIGN KEY (clasificacion_id, concepto_id) REFERENCES public.clasificacion(id, concepto_id);


--
-- Name: movimiento_bancario movimiento_bancario_concepto_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.movimiento_bancario
    ADD CONSTRAINT movimiento_bancario_concepto_id_fkey FOREIGN KEY (concepto_id) REFERENCES public.concepto(id);


--
-- Name: movimiento_bancario movimiento_bancario_cuenta_bancaria_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.movimiento_bancario
    ADD CONSTRAINT movimiento_bancario_cuenta_bancaria_id_fkey FOREIGN KEY (cuenta_bancaria_id) REFERENCES public.cuenta_bancaria(id);


--
-- Name: movimiento_bancario movimiento_bancario_documento_cxp_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.movimiento_bancario
    ADD CONSTRAINT movimiento_bancario_documento_cxp_id_fkey FOREIGN KEY (documento_cxp_id) REFERENCES public.documento_cxp(id);


--
-- Name: movimiento_bancario movimiento_bancario_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.movimiento_bancario
    ADD CONSTRAINT movimiento_bancario_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: movimiento_bancario movimiento_bancario_importacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.movimiento_bancario
    ADD CONSTRAINT movimiento_bancario_importacion_id_fkey FOREIGN KEY (importacion_id) REFERENCES public.importacion(id);


--
-- Name: movimiento_bancario movimiento_bancario_par_traslado_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.movimiento_bancario
    ADD CONSTRAINT movimiento_bancario_par_traslado_id_fkey FOREIGN KEY (par_traslado_id) REFERENCES public.movimiento_bancario(id);


--
-- Name: nomina_archivo_pago nomina_archivo_pago_corrida_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nomina_archivo_pago
    ADD CONSTRAINT nomina_archivo_pago_corrida_id_fkey FOREIGN KEY (corrida_id) REFERENCES public.corrida_nomina(id);


--
-- Name: nomina_archivo_pago nomina_archivo_pago_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nomina_archivo_pago
    ADD CONSTRAINT nomina_archivo_pago_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: nomina_parametros nomina_parametros_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nomina_parametros
    ADD CONSTRAINT nomina_parametros_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- Name: nota_credito_aplicacion nota_credito_aplicacion_cargo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_aplicacion
    ADD CONSTRAINT nota_credito_aplicacion_cargo_id_fkey FOREIGN KEY (cargo_id) REFERENCES public.cargo_cxc(id);


--
-- Name: nota_credito_aplicacion nota_credito_aplicacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_aplicacion
    ADD CONSTRAINT nota_credito_aplicacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: nota_credito_aplicacion nota_credito_aplicacion_nota_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_aplicacion
    ADD CONSTRAINT nota_credito_aplicacion_nota_id_fkey FOREIGN KEY (nota_id) REFERENCES public.nota_credito_cxc(id) ON DELETE CASCADE;


--
-- Name: nota_credito_cxc nota_credito_cxc_anulada_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_cxc
    ADD CONSTRAINT nota_credito_cxc_anulada_por_fkey FOREIGN KEY (anulada_por) REFERENCES public.usuario(id);


--
-- Name: nota_credito_cxc nota_credito_cxc_cargo_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_cxc
    ADD CONSTRAINT nota_credito_cxc_cargo_id_fkey FOREIGN KEY (cargo_id) REFERENCES public.cargo_cxc(id);


--
-- Name: nota_credito_cxc nota_credito_cxc_contrato_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_cxc
    ADD CONSTRAINT nota_credito_cxc_contrato_id_fkey FOREIGN KEY (contrato_id) REFERENCES public.contrato_cxc(id);


--
-- Name: nota_credito_cxc nota_credito_cxc_creado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_cxc
    ADD CONSTRAINT nota_credito_cxc_creado_por_fkey FOREIGN KEY (creado_por) REFERENCES public.usuario(id);


--
-- Name: nota_credito_cxc nota_credito_cxc_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nota_credito_cxc
    ADD CONSTRAINT nota_credito_cxc_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: palabra_clave palabra_clave_regla_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.palabra_clave
    ADD CONSTRAINT palabra_clave_regla_id_fkey FOREIGN KEY (regla_id) REFERENCES public.regla_clasificacion(id) ON DELETE CASCADE;


--
-- Name: partida_conciliacion partida_conciliacion_anulada_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.partida_conciliacion
    ADD CONSTRAINT partida_conciliacion_anulada_por_fkey FOREIGN KEY (anulada_por) REFERENCES public.usuario(id);


--
-- Name: partida_conciliacion partida_conciliacion_cuenta_bancaria_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.partida_conciliacion
    ADD CONSTRAINT partida_conciliacion_cuenta_bancaria_id_fkey FOREIGN KEY (cuenta_bancaria_id) REFERENCES public.cuenta_bancaria(id);


--
-- Name: partida_conciliacion partida_conciliacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.partida_conciliacion
    ADD CONSTRAINT partida_conciliacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: partida_conciliacion partida_conciliacion_registrado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.partida_conciliacion
    ADD CONSTRAINT partida_conciliacion_registrado_por_fkey FOREIGN KEY (registrado_por) REFERENCES public.usuario(id);


--
-- Name: periodo_cierre periodo_cierre_cerrado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.periodo_cierre
    ADD CONSTRAINT periodo_cierre_cerrado_por_fkey FOREIGN KEY (cerrado_por) REFERENCES public.usuario(id);


--
-- Name: periodo_cierre periodo_cierre_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.periodo_cierre
    ADD CONSTRAINT periodo_cierre_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: plantilla_correo plantilla_correo_actualizado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plantilla_correo
    ADD CONSTRAINT plantilla_correo_actualizado_por_fkey FOREIGN KEY (actualizado_por) REFERENCES public.usuario(id);


--
-- Name: plantilla_correo plantilla_correo_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plantilla_correo
    ADD CONSTRAINT plantilla_correo_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: promesa_pago_cxc promesa_pago_cxc_contrato_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promesa_pago_cxc
    ADD CONSTRAINT promesa_pago_cxc_contrato_id_fkey FOREIGN KEY (contrato_id) REFERENCES public.contrato_cxc(id) ON DELETE CASCADE;


--
-- Name: promesa_pago_cxc promesa_pago_cxc_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promesa_pago_cxc
    ADD CONSTRAINT promesa_pago_cxc_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: promesa_pago_cxc promesa_pago_cxc_gestion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.promesa_pago_cxc
    ADD CONSTRAINT promesa_pago_cxc_gestion_id_fkey FOREIGN KEY (gestion_id) REFERENCES public.gestion_cxc(id) ON DELETE CASCADE;


--
-- Name: proveedor proveedor_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor
    ADD CONSTRAINT proveedor_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: proveedor proveedor_gasto_clasificacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor
    ADD CONSTRAINT proveedor_gasto_clasificacion_id_fkey FOREIGN KEY (gasto_clasificacion_id) REFERENCES public.clasificacion(id);


--
-- Name: proveedor_gasto proveedor_gasto_clasificacion_id_fkey1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor_gasto
    ADD CONSTRAINT proveedor_gasto_clasificacion_id_fkey1 FOREIGN KEY (clasificacion_id) REFERENCES public.clasificacion(id);


--
-- Name: proveedor proveedor_gasto_concepto_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor
    ADD CONSTRAINT proveedor_gasto_concepto_id_fkey FOREIGN KEY (gasto_concepto_id) REFERENCES public.concepto(id);


--
-- Name: proveedor_gasto proveedor_gasto_concepto_id_fkey1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor_gasto
    ADD CONSTRAINT proveedor_gasto_concepto_id_fkey1 FOREIGN KEY (concepto_id) REFERENCES public.concepto(id);


--
-- Name: proveedor_gasto proveedor_gasto_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor_gasto
    ADD CONSTRAINT proveedor_gasto_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: proveedor_gasto proveedor_gasto_proveedor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor_gasto
    ADD CONSTRAINT proveedor_gasto_proveedor_id_fkey FOREIGN KEY (proveedor_id) REFERENCES public.proveedor(id) ON DELETE CASCADE;


--
-- Name: proveedor proveedor_gasto_subclasificacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor
    ADD CONSTRAINT proveedor_gasto_subclasificacion_id_fkey FOREIGN KEY (gasto_subclasificacion_id) REFERENCES public.subclasificacion(id);


--
-- Name: proveedor_gasto proveedor_gasto_subclasificacion_id_fkey1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proveedor_gasto
    ADD CONSTRAINT proveedor_gasto_subclasificacion_id_fkey1 FOREIGN KEY (subclasificacion_id) REFERENCES public.subclasificacion(id);


--
-- Name: proyeccion_escenario proyeccion_escenario_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.proyeccion_escenario
    ADD CONSTRAINT proyeccion_escenario_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: regla_clasificacion regla_clasificacion_clasificacion_id_concepto_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.regla_clasificacion
    ADD CONSTRAINT regla_clasificacion_clasificacion_id_concepto_id_fkey FOREIGN KEY (clasificacion_id, concepto_id) REFERENCES public.clasificacion(id, concepto_id);


--
-- Name: regla_clasificacion regla_clasificacion_concepto_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.regla_clasificacion
    ADD CONSTRAINT regla_clasificacion_concepto_id_fkey FOREIGN KEY (concepto_id) REFERENCES public.concepto(id);


--
-- Name: regla_clasificacion regla_clasificacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.regla_clasificacion
    ADD CONSTRAINT regla_clasificacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: rol rol_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol
    ADD CONSTRAINT rol_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: rol_permiso rol_permiso_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol_permiso
    ADD CONSTRAINT rol_permiso_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: rol_permiso rol_permiso_permiso_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol_permiso
    ADD CONSTRAINT rol_permiso_permiso_id_fkey FOREIGN KEY (permiso_id) REFERENCES public.permiso(id) ON DELETE CASCADE;


--
-- Name: rol_permiso rol_permiso_rol_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rol_permiso
    ADD CONSTRAINT rol_permiso_rol_id_fkey FOREIGN KEY (rol_id) REFERENCES public.rol(id) ON DELETE CASCADE;


--
-- Name: saldo_cuenta_diario saldo_cuenta_diario_capturado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saldo_cuenta_diario
    ADD CONSTRAINT saldo_cuenta_diario_capturado_por_fkey FOREIGN KEY (capturado_por) REFERENCES public.usuario(id);


--
-- Name: saldo_cuenta_diario saldo_cuenta_diario_cuenta_bancaria_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saldo_cuenta_diario
    ADD CONSTRAINT saldo_cuenta_diario_cuenta_bancaria_id_fkey FOREIGN KEY (cuenta_bancaria_id) REFERENCES public.cuenta_bancaria(id);


--
-- Name: saldo_cuenta_diario saldo_cuenta_diario_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saldo_cuenta_diario
    ADD CONSTRAINT saldo_cuenta_diario_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: saldo_cuenta_diario saldo_cuenta_diario_revisado_por_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.saldo_cuenta_diario
    ADD CONSTRAINT saldo_cuenta_diario_revisado_por_fkey FOREIGN KEY (revisado_por) REFERENCES public.usuario(id);


--
-- Name: sesion sesion_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sesion
    ADD CONSTRAINT sesion_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: subclasificacion subclasificacion_clasificacion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subclasificacion
    ADD CONSTRAINT subclasificacion_clasificacion_id_fkey FOREIGN KEY (clasificacion_id) REFERENCES public.clasificacion(id) ON DELETE CASCADE;


--
-- Name: subclasificacion subclasificacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subclasificacion
    ADD CONSTRAINT subclasificacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: tipo_cambio_cotizacion tipo_cambio_cotizacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tipo_cambio_cotizacion
    ADD CONSTRAINT tipo_cambio_cotizacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: tipo_cambio_mes tipo_cambio_mes_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tipo_cambio_mes
    ADD CONSTRAINT tipo_cambio_mes_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: usuario_empresa_rol usuario_empresa_rol_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usuario_empresa_rol
    ADD CONSTRAINT usuario_empresa_rol_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id) ON DELETE CASCADE;


--
-- Name: usuario_empresa_rol usuario_empresa_rol_rol_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usuario_empresa_rol
    ADD CONSTRAINT usuario_empresa_rol_rol_id_fkey FOREIGN KEY (rol_id) REFERENCES public.rol(id);


--
-- Name: usuario_empresa_rol usuario_empresa_rol_usuario_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usuario_empresa_rol
    ADD CONSTRAINT usuario_empresa_rol_usuario_id_fkey FOREIGN KEY (usuario_id) REFERENCES public.usuario(id) ON DELETE CASCADE;


--
-- Name: vacacion vacacion_empleado_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vacacion
    ADD CONSTRAINT vacacion_empleado_id_fkey FOREIGN KEY (empleado_id) REFERENCES public.empleado(id);


--
-- Name: vacacion vacacion_empresa_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vacacion
    ADD CONSTRAINT vacacion_empresa_id_fkey FOREIGN KEY (empresa_id) REFERENCES public.empresa(id);


--
-- PostgreSQL database dump complete
--

\unrestrict XeXZotFf7DC7oWQGqvydDNY2euUWx8DsUWDlUgitEzeh9oepTCqsuUidpEnriFq

