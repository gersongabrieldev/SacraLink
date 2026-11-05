// Textos da interface. Português do Brasil é o idioma padrão; a estrutura
// permite adicionar outros idiomas (ex.: 'en') sem alterar os componentes.

const ptBR = {
  'state.OFFLINE': 'Offline',
  'state.CONNECTING': 'Conectando',
  'state.CONNECTED': 'Conectada',
  'state.STREAMING': 'Transmitindo',
  'state.RECONNECTING': 'Reconectando',
  'state.ERROR': 'Erro',
  'tally.on_air': 'NO AR',
  'tally.preview': 'PREVIEW',
  'tally.none': 'Fora do ar',
  'tally.unknown': 'OBS sem conexão',
  'obs.disabled': 'OBS não configurado',
  'obs.disconnected': 'OBS desconectado',
  'obs.connecting': 'Conectando ao OBS',
  'obs.connected': 'OBS conectado',
  'obs.auth_failed': 'Senha do OBS incorreta',
  'media.running': 'Servidor de mídia ativo',
  'media.starting': 'Servidor de mídia iniciando',
  'media.restarting': 'Servidor de mídia reiniciando',
  'media.failed': 'Servidor de mídia com falha',
  'media.stopped': 'Servidor de mídia parado',
  'nav.dashboard': 'Produção',
  'nav.devices': 'Câmeras e aparelhos',
  'nav.obs': 'OBS e áudio',
  'nav.network': 'Rede',
  'nav.settings': 'Configurações',
  'nav.events': 'Eventos',
  'cmd.executed': 'executado',
  'cmd.rejected': 'recusado pelo aparelho',
  'cmd.unsupported': 'não suportado pelo aparelho',
  'cmd.failed': 'falhou no aparelho',
  'cmd.conflict': 'o operador do celular alterou agora — tente de novo',
  'cmd.expired': 'expirado',
  'cmd.timeout': 'sem resposta do aparelho',
  'cmd.offline': 'aparelho desconectado',
} as const;

export type MessageKey = keyof typeof ptBR;

const dictionaries: Record<string, Partial<Record<MessageKey, string>>> = { 'pt-BR': ptBR };

let current = 'pt-BR';

export function setLanguage(lang: string) {
  if (dictionaries[lang]) current = lang;
}

export function t(key: MessageKey | string): string {
  const d = dictionaries[current] as Record<string, string>;
  return d[key] ?? (ptBR as Record<string, string>)[key] ?? key;
}
