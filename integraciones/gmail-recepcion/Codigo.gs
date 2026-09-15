/**
 * ============================================================================
 *  CONECTOR DE RECEPCIÓN DE FACTURAS  ·  Gmail → GPVDP ERP
 * ----------------------------------------------------------------------------
 *  Lee los comprobantes electrónicos que llegan al buzón de la empresa y los
 *  entrega al ERP por HTTPS. No escribe en Google Sheets: el ERP es el sistema
 *  de registro.
 *
 *  Reemplaza al extractor que volcaba los XML en una hoja para después subir un
 *  .xlsx a mano.
 * ============================================================================
 *
 * ── LA REGLA MÁS IMPORTANTE DE ESTE ARCHIVO ─────────────────────────────────
 *
 * EL HILO SE ETIQUETA `Procesado-XML` SOLO DESPUÉS DE UN 2xx. Nunca antes.
 *
 * El extractor anterior etiquetaba primero y enviaba después. Si el envío
 * fallaba —el ERP reiniciándose, un timeout, la red de Google—, la corrida
 * siguiente ya no encontraba el hilo, porque la búsqueda excluye lo etiquetado.
 * El correo seguía en Gmail para siempre, pero quedaba INVISIBLE: nadie tiene
 * motivo para buscar ese correo entre miles, el ERP nunca supo que existió, y
 * no había ninguna lista que dijera «faltan estas cinco facturas».
 *
 * Acá, un envío que no sale bien deja el hilo SIN etiquetar (así vuelve solo en
 * la corrida siguiente) y le pone `Error-Envio` para que se vea. Reintentar es
 * seguro: el ERP deduplica por el Message-ID del correo más la clave del
 * comprobante, y contesta 200 con `repetido: true` cuando ya lo tenía.
 *
 * ── LO QUE NO VA EN ESTE CÓDIGO ─────────────────────────────────────────────
 *
 * La credencial NO se escribe acá. Vive en las propiedades del script
 * (Configuración del proyecto › Propiedades del script), porque este archivo se
 * copia, se comparte y se versiona, y un token pegado adentro se filtra solo.
 * Ver README.md, paso 4.
 */

/* ══════════════════════════════ CONFIGURACIÓN ═════════════════════════════ */

/** Nombres de las propiedades del script. No son valores: son las llaves. */
var PROP = {
  URL: 'GPVDP_URL', // https://finanzas.corporativovdp.com
  TOKEN: 'GPVDP_TOKEN', // la credencial del buzón (pantalla CxP › Buzones)
  DESDE: 'GPVDP_DESDE', // opcional: 'AAAA/MM/DD' para el arranque en frío
};

var CFG = {
  ETIQUETA_OK: 'Procesado-XML',
  ETIQUETA_ERROR: 'Error-Envio',

  /**
   * Hilos por corrida. Apps Script corta la ejecución a los 6 minutos (30 en
   * Workspace), y un corte a mitad de camino es inofensivo acá —lo no enviado
   * queda sin etiquetar y entra en la corrida siguiente— pero conviene no
   * provocarlo: con 20 hilos hay margen de sobra.
   */
  MAX_HILOS: 20,

  /** Topes del ERP. Enviar algo más grande sería un viaje perdido. */
  MAX_XML: 4 * 1024 * 1024,
  MAX_PDF: 12 * 1024 * 1024,

  /** Reintentos ante un error de SISTEMA (5xx o red). Con espera creciente. */
  REINTENTOS: 3,

  HOJA_BITACORA: 'Bitácora de envíos',
};

/* ═══════════════════════════════ MENÚ ═════════════════════════════════════ */

function onOpen() {
  SpreadsheetApp.getUi()
    .createMenu('📤 Recepción GPVDP')
    .addItem('▶ Procesar correos pendientes', 'procesarPendientes')
    .addItem('✔ Verificar la configuración', 'verificarConfiguracion')
    .addItem('⏱ Instalar el disparador cada hora', 'instalarDisparador')
    .addItem('🩺 Enviar solo el latido', 'enviarSoloLatido')
    .addToUi();
}

/* ═════════════════════════ PROCESO PRINCIPAL ══════════════════════════════ */

/**
 * Lee los correos pendientes y entrega cada comprobante al ERP.
 *
 * Es la función que corre el disparador. Con el candado, dos corridas solapadas
 * no se pisan; y aunque se pisaran, el ERP deduplica.
 */
function procesarPendientes() {
  var lock = LockService.getScriptLock();
  if (!lock.tryLock(30000)) {
    log_('Otra corrida en curso: se omite.');
    return;
  }
  try {
    var cfg = leerConfig_();
    var buzon = buzonActual_();

    var query =
      'has:attachment filename:xml -label:' + CFG.ETIQUETA_OK +
      (cfg.desde ? ' after:' + cfg.desde : '');
    var hilos = GmailApp.search(query, 0, CFG.MAX_HILOS);

    // EL LATIDO, SIEMPRE. Es lo único que le permite al ERP distinguir «hoy no
    // hubo facturas» de «el conector está muerto». Sin esto, que Google
    // desactive el disparador o que alguien revoque la credencial se ve en
    // pantalla igual que un día tranquilo, y las facturas aparecen tres semanas
    // después, varias ya vencidas.
    latido_(cfg, buzon);

    if (!hilos.length) {
      log_('Sin correos pendientes. Latido enviado.');
      return;
    }

    var lblOk = etiqueta_(CFG.ETIQUETA_OK);
    var lblErr = etiqueta_(CFG.ETIQUETA_ERROR);
    var totales = { enviados: 0, repetidos: 0, parqueados: 0, fallados: 0, hilosOk: 0 };
    var filas = [];

    for (var h = 0; h < hilos.length; h++) {
      var hilo = hilos[h];
      var todoEntregado = true;
      var huboAlgo = false;

      var mensajes = hilo.getMessages();
      for (var m = 0; m < mensajes.length; m++) {
        var msg = mensajes[m];
        var adjuntos = msg.getAttachments({ includeInlineImages: false });
        var xmls = filtrarPorExtension_(adjuntos, '.xml');

        for (var x = 0; x < xmls.length; x++) {
          huboAlgo = true;
          var xml = xmls[x];

          if (xml.getBytes().length > CFG.MAX_XML) {
            todoEntregado = false;
            totales.fallados++;
            filas.push(fila_(msg, xml.getName(), 'NO ENVIADO', 'el XML pesa más de 4 MB'));
            continue;
          }

          var pdf = emparejarPdf_(adjuntos, xml.getName());
          var res = enviarComprobante_(cfg, buzon, msg, xml, pdf);

          if (!res.ok) {
            todoEntregado = false;
            totales.fallados++;
            filas.push(fila_(msg, xml.getName(), 'ERROR ' + res.codigo, res.detalle));
            continue;
          }

          // 2xx. El ERP ya lo tiene guardado, aunque no haya podido registrarlo
          // como cuenta por pagar: eso queda en su cola de errores, no acá.
          if (res.cuerpo.repetido) totales.repetidos++;
          else if (res.cuerpo.estado === 'PROCESADA') totales.enviados++;
          else totales.parqueados++;

          filas.push(
            fila_(msg, xml.getName(),
              res.cuerpo.estado + (res.cuerpo.repetido ? ' (repetido)' : ''),
              res.cuerpo.motivo || res.cuerpo.consecutivo || '',
              res.cuerpo.clave, pdf ? pdf.getName() : ''));
        }
      }

      // ── ACÁ SE DECIDE SI EL HILO SE MARCA COMO PROCESADO ──────────────────
      //
      // Solo si TODOS sus comprobantes salieron con 2xx. Si uno falló, el hilo
      // queda sin la etiqueta y la corrida siguiente lo vuelve a tomar completo:
      // los que ya entraron se resuelven como repetidos y el que faltaba se
      // envía. Nada se duplica y nada se pierde.
      if (huboAlgo && todoEntregado) {
        hilo.addLabel(lblOk);
        hilo.removeLabel(lblErr); // por si venía marcado de una corrida anterior
        totales.hilosOk++;
      } else if (huboAlgo) {
        hilo.addLabel(lblErr);
      }
    }

    escribirBitacora_(filas);
    log_(
      'Hilos: ' + hilos.length + ' (completos ' + totales.hilosOk + ') · ' +
      'registradas ' + totales.enviados + ' · repetidas ' + totales.repetidos + ' · ' +
      'en cola del ERP ' + totales.parqueados + ' · fallidas ' + totales.fallados);
  } finally {
    lock.releaseLock();
  }
}

/* ════════════════════════════ ENVÍO AL ERP ════════════════════════════════ */

/**
 * Entrega un comprobante. Devuelve {ok, codigo, detalle, cuerpo}.
 *
 * Reintenta solo los errores de SISTEMA (5xx y fallas de red), porque son los
 * que se arreglan solos. Un 401 (credencial mala) o un 400 (petición mal
 * armada) no mejoran reintentando: se reportan y el hilo queda sin etiquetar,
 * así que vuelven cuando el problema esté resuelto.
 */
function enviarComprobante_(cfg, buzon, msg, xml, pdf) {
  var payload = {
    xml: xml.copyBlob().setName(xml.getName() || 'comprobante.xml'),
    message_id: msg.getId(),
    asunto: msg.getSubject() || '',
    remitente: msg.getFrom() || '',
    // El buzón NO se escribe a mano: sale de la cuenta que corre el script. Por
    // eso el cotejo del ERP sirve de algo — si alguien pegó la credencial de
    // otra empresa, el buzón no va a calzar y el ERP lo dice.
    buzon: buzon,
  };
  if (pdf) payload.pdf = pdf.copyBlob().setName(pdf.getName() || 'comprobante.pdf');

  var opciones = {
    method: 'post',
    payload: payload, // con Blobs adentro, Apps Script arma el multipart solo
    headers: { Authorization: 'Bearer ' + cfg.token },
    muteHttpExceptions: true,
    followRedirects: false,
  };

  var espera = 2000;
  for (var intento = 1; intento <= CFG.REINTENTOS; intento++) {
    var codigo = 0;
    var texto = '';
    try {
      var r = UrlFetchApp.fetch(cfg.url + '/v1/cxp/recepcion', opciones);
      codigo = r.getResponseCode();
      texto = r.getContentText();
    } catch (e) {
      // Falla de red: se trata como 5xx.
      codigo = 0;
      texto = String(e);
    }

    if (codigo >= 200 && codigo < 300) {
      var cuerpo = {};
      try { cuerpo = JSON.parse(texto); } catch (e) { cuerpo = {}; }
      return { ok: true, codigo: codigo, detalle: '', cuerpo: cuerpo };
    }

    var esDeSistema = codigo === 0 || codigo >= 500 || codigo === 429;
    if (!esDeSistema || intento === CFG.REINTENTOS) {
      return { ok: false, codigo: codigo, detalle: recortar_(texto), cuerpo: {} };
    }
    Utilities.sleep(espera);
    espera = espera * 2;
  }
  return { ok: false, codigo: 0, detalle: 'sin respuesta', cuerpo: {} };
}

/**
 * Avisa al ERP que el conector corrió. Se manda SIEMPRE, con facturas o sin
 * ellas, y su falla no detiene la corrida: es telemetría, no transporte.
 */
function latido_(cfg, buzon) {
  try {
    UrlFetchApp.fetch(cfg.url + '/v1/cxp/recepcion/latido', {
      method: 'post',
      headers: { Authorization: 'Bearer ' + cfg.token },
      payload: { buzon: buzon },
      muteHttpExceptions: true,
    });
  } catch (e) {
    log_('El latido no salió: ' + e);
  }
}

/* ═══════════════════════ EMPAREJAR EL PDF CON SU XML ══════════════════════ */

/**
 * Busca la representación gráfica que corresponde a este XML.
 *
 * Se prueba de lo más seguro a lo más flojo, y si hay duda NO se adivina: es
 * mejor entregar la factura sin PDF —el XML es el dato con valor legal— que
 * pegarle el PDF de otra factura y que alguien pague mirando el documento
 * equivocado.
 *
 *   1. mismo nombre base («506….xml» y «506….pdf»);
 *   2. el nombre del PDF contiene la clave o el nombre base del XML;
 *   3. hay exactamente UN XML y UN PDF en el mensaje: se emparejan.
 */
function emparejarPdf_(adjuntos, nombreXml) {
  var pdfs = filtrarPorExtension_(adjuntos, '.pdf');
  if (!pdfs.length) return null;

  var base = String(nombreXml || '').replace(/\.xml$/i, '');

  for (var i = 0; i < pdfs.length; i++) {
    if (pdfs[i].getName().replace(/\.pdf$/i, '') === base) return acotarPdf_(pdfs[i]);
  }
  if (base.length >= 8) {
    for (var j = 0; j < pdfs.length; j++) {
      if (pdfs[j].getName().indexOf(base) !== -1) return acotarPdf_(pdfs[j]);
    }
  }
  var xmls = filtrarPorExtension_(adjuntos, '.xml');
  if (xmls.length === 1 && pdfs.length === 1) return acotarPdf_(pdfs[0]);

  return null; // varios de cada uno y sin forma de emparejar: se manda solo el XML
}

/** El PDF es opcional: si excede el tope del ERP se omite, no se pierde la factura. */
function acotarPdf_(pdf) {
  return pdf.getBytes().length > CFG.MAX_PDF ? null : pdf;
}

function filtrarPorExtension_(adjuntos, ext) {
  var out = [];
  for (var i = 0; i < adjuntos.length; i++) {
    var n = String(adjuntos[i].getName() || '').toLowerCase();
    if (n.length >= ext.length && n.slice(-ext.length) === ext) out.push(adjuntos[i]);
  }
  return out;
}

/* ════════════════════════════ CONFIGURACIÓN ═══════════════════════════════ */

/** Lee la configuración de las propiedades del script y falla claro si falta algo. */
function leerConfig_() {
  var p = PropertiesService.getScriptProperties();
  var url = (p.getProperty(PROP.URL) || '').replace(/\/+$/, '');
  var token = p.getProperty(PROP.TOKEN) || '';
  var desde = p.getProperty(PROP.DESDE) || '';

  if (!url || !token) {
    throw new Error(
      'Falta configurar el conector. En «Configuración del proyecto › Propiedades del script» ' +
      'agregá ' + PROP.URL + ' y ' + PROP.TOKEN + '. Ver README.md, paso 4.');
  }
  if (url.indexOf('https://') !== 0) {
    // La credencial viaja en la cabecera: sin TLS va en claro por la red.
    throw new Error('La URL tiene que empezar con https:// — la credencial viaja en la cabecera.');
  }
  return { url: url, token: token, desde: desde };
}

/** El buzón real: la cuenta de Google que corre este script. */
function buzonActual_() {
  var e = '';
  try { e = Session.getActiveUser().getEmail() || ''; } catch (err) { e = ''; }
  if (!e) {
    try { e = Session.getEffectiveUser().getEmail() || ''; } catch (err2) { e = ''; }
  }
  return e;
}

/**
 * Comprueba la configuración de punta a punta SIN mover ningún correo.
 *
 * Usa el latido, que es una escritura inofensiva: si contesta, la URL, la
 * credencial y el permiso de red están bien.
 */
function verificarConfiguracion() {
  var ui = SpreadsheetApp.getUi();
  var cfg;
  try {
    cfg = leerConfig_();
  } catch (e) {
    ui.alert('Configuración incompleta', String(e.message || e), ui.ButtonSet.OK);
    return;
  }
  var buzon = buzonActual_();
  var r;
  try {
    r = UrlFetchApp.fetch(cfg.url + '/v1/cxp/recepcion/latido', {
      method: 'post',
      headers: { Authorization: 'Bearer ' + cfg.token },
      payload: { buzon: buzon },
      muteHttpExceptions: true,
    });
  } catch (e) {
    ui.alert('No se pudo llegar al ERP', String(e), ui.ButtonSet.OK);
    return;
  }

  var codigo = r.getResponseCode();
  var cuerpo = r.getContentText();
  if (codigo === 200) {
    var buzonERP = '';
    try { buzonERP = JSON.parse(cuerpo).buzon || ''; } catch (e) { buzonERP = ''; }
    var aviso = 'Conexión correcta.\n\nBuzón de este script: ' + buzon +
      '\nBuzón configurado en el ERP: ' + buzonERP;
    // Si no coinciden, las facturas van a entrar pero el ERP las va a parquear
    // diciendo que el token no corresponde al buzón. Vale avisarlo ACÁ.
    if (buzonERP && buzon && buzonERP.toLowerCase() !== buzon.toLowerCase()) {
      aviso += '\n\n⚠ NO COINCIDEN. Esta credencial es de otro buzón: revisá que no sea la de ' +
        'otra empresa. Con esto, el ERP va a rechazar todo lo que se envíe.';
    }
    ui.alert('Verificación', aviso, ui.ButtonSet.OK);
    return;
  }
  if (codigo === 401) {
    ui.alert('Credencial inválida',
      'El ERP no reconoce la credencial (401). Puede estar rotada o el buzón desactivado. ' +
      'Generá una nueva en CxP › Buzones y pegala en las propiedades del script.',
      ui.ButtonSet.OK);
    return;
  }
  ui.alert('Respuesta inesperada', 'HTTP ' + codigo + '\n' + recortar_(cuerpo), ui.ButtonSet.OK);
}

/** Deja el disparador cada hora, sin duplicarlo si ya existía. */
function instalarDisparador() {
  var ui = SpreadsheetApp.getUi();
  var existentes = ScriptApp.getProjectTriggers();
  for (var i = 0; i < existentes.length; i++) {
    if (existentes[i].getHandlerFunction() === 'procesarPendientes') {
      ScriptApp.deleteTrigger(existentes[i]);
    }
  }
  ScriptApp.newTrigger('procesarPendientes').timeBased().everyHours(1).create();
  ui.alert('Disparador instalado',
    'El conector va a correr cada hora. Podés seguirlo en «Ejecuciones» del editor de Apps Script, ' +
    'y en el ERP en CxP › Buzones (columna «Última señal»).', ui.ButtonSet.OK);
}

function enviarSoloLatido() {
  var cfg = leerConfig_();
  latido_(cfg, buzonActual_());
  SpreadsheetApp.getUi().alert('Latido enviado. Revisalo en CxP › Buzones.');
}

/* ══════════════════════════════ BITÁCORA ══════════════════════════════════ */

/**
 * Registro local de lo enviado.
 *
 * No es decorativo: el ERP guarda el Message-ID de cada recepción, así que esta
 * bitácora es la otra mitad de la conciliación —«qué mandé» contra «qué tiene el
 * ERP»—. Si algún día falta una factura, la diferencia entre las dos listas dice
 * exactamente cuál.
 */
function fila_(msg, nombreXml, estado, detalle, clave, nombrePdf) {
  return [
    new Date(),
    msg.getId(),
    msg.getDate(),
    msg.getFrom(),
    msg.getSubject(),
    nombreXml || '',
    nombrePdf || '',
    clave || '',
    estado || '',
    detalle || '',
  ];
}

function escribirBitacora_(filas) {
  if (!filas.length) return;
  var ss = SpreadsheetApp.getActiveSpreadsheet();
  var hoja = ss.getSheetByName(CFG.HOJA_BITACORA);
  if (!hoja) {
    hoja = ss.insertSheet(CFG.HOJA_BITACORA);
    hoja.appendRow([
      'Enviado', 'ID del mensaje', 'Fecha del correo', 'Remitente', 'Asunto',
      'XML', 'PDF', 'Clave', 'Resultado', 'Detalle',
    ]);
    hoja.getRange(1, 1, 1, 10).setFontWeight('bold').setBackground('#1F3864').setFontColor('#FFFFFF');
    hoja.setFrozenRows(1);
  }
  hoja.getRange(hoja.getLastRow() + 1, 1, filas.length, 10).setValues(filas);
}

/* ═══════════════════════════════ UTILIDADES ═══════════════════════════════ */

function etiqueta_(nombre) {
  return GmailApp.getUserLabelByName(nombre) || GmailApp.createLabel(nombre);
}

function recortar_(s) {
  s = String(s || '');
  return s.length > 300 ? s.slice(0, 300) + '…' : s;
}

function log_(m) {
  Logger.log(m);
}
