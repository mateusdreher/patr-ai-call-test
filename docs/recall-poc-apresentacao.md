# Recall.ai POC

## Objetivo

Validar se a Recall.ai atende o caso de uso da plataforma:

- capturar reunioes automaticamente;
- operar com um email/agenda dedicados para gravação;
- saber quem participou da call;
- obter audio e transcript por API;
- processar tudo internamente na nossa plataforma depois da reuniao.

## Resumo executivo

A Recall.ai atende bem a parte de captura e normalizacao de reunioes em multiplas plataformas. O ponto central da arquitetura e que a Recall **nao decide sozinha** entrar em uma reuniao. Ela precisa receber esse contexto via API ou via integracao de calendario.

Para esta avaliacao, a abordagem escolhida e usar o **calendar sync da Recall** como mecanismo principal de entrada nas reunioes.

O fluxo-base fica assim:

1. manter um email/agenda dedicados para as reunioes que queremos capturar;
2. sincronizar essa agenda com a Recall;
3. agendar bots para os eventos relevantes do calendario;
4. consumir webhooks para acompanhar o lifecycle;
5. ao final, buscar transcript, audio, metadata e eventos de participantes;
6. enviar esses artefatos para o pipeline interno da plataforma.

## Entrada em reunioes via calendar sync da Recall

A Recall entra em reunioes quando existe um bot associado a um evento de calendario ou a uma `meeting_url` conhecida. Neste projeto, a decisao e usar a Calendar Integration da Recall para que o lifecycle da reuniao seja guiado pela agenda, e nao por chamadas avulsas de `Create Bot`.

Isso significa que a plataforma nao precisa "descobrir" a reuniao em tempo real. Ela precisa garantir que o evento existe na agenda conectada, que esse evento foi sincronizado pela Recall e que o bot foi agendado para ele.

### Como fazer isso de fato

O caminho pratico para operar com calendar sync da Recall e:

1. Criar um email dedicado da operacao.
2. Conectar esse usuario na Calendar Integration da Recall.
3. Garantir que as reunioes que queremos capturar convidem esse email.
4. Sincronizar os eventos desse calendario com a Recall.
5. Para cada evento relevante, agendar o bot usando os endpoints de calendar event da Recall.
6. Receber webhooks de mudanca de estado para saber quando o bot entrou, quando a gravacao comecou e quando os artefatos finais ficaram prontos.
7. Ao final, buscar transcript, audio, metadata e participant events para processamento interno.

Em outras palavras:

- a agenda dedicada e o ponto de entrada operacional;
- a Recall usa essa agenda para manter o contexto da reuniao;
- a plataforma continua responsavel por decidir quais eventos devem ou nao ser gravados.

## Cancelamento, remocao e delecao de dados

Quando a reuniao deixa de ser relevante para captura, existem tres niveis diferentes de remocao e eles nao devem ser confundidos:

- cancelamento de um bot ainda agendado para uma reuniao futura;
- remocao de um bot de uma reuniao que ja esta entrando ou em andamento;
- delecao dos dados capturados pela Recall apos o processamento interno.

Na pratica, isso significa:

- se a reuniao foi cancelada antes de acontecer, o bot pode ser desagendado;
- se a reuniao ja comecou e o bot precisa sair, ele pode ser removido da call;
- se transcript, audio e metadata ja foram consumidos pela nossa plataforma, a midia pode ser apagada da Recall.

O que nao acontece pela Recall e a delecao da reuniao original no Teams, Meet ou Zoom. O evento de calendario e a reuniao continuam pertencendo ao provedor.

Portanto, o termo correto aqui nao e "deletar reuniao", e sim:

- desagendar captura;
- remover o bot da call;
- apagar os artefatos capturados.

## Identidade de participantes: nome e email

A Recall normalmente consegue fornecer o **nome** do participante nos metadados da reuniao e nos recursos ligados a transcript e participant events.

Ja o **email** nao e exposto diretamente pela plataforma de videoconferencia. Pela documentacao oficial, a Recall tenta preencher o campo `email` fazendo matching entre:

- os attendees do evento de calendario;
- os nomes dos participantes que efetivamente aparecem na call.

Esse preenchimento depende de alguns requisitos:

- o bot precisa ter sido criado via Calendar Integration da Recall;
- a feature de participant emails precisa estar habilitada na conta/workspace;
- o evento de calendario precisa conter os convidados individualmente;
- os nomes mostrados na reuniao precisam permitir matching com os nomes do calendario.

Por isso, o email deve ser tratado como um dado **derivado**, e nao como um campo nativo e garantido da chamada.

### Cenario de mesma organizacao no Teams

Se todos os participantes usam email corporativo da mesma organizacao e a agenda tambem pertence a essa organizacao, isso melhora bastante a chance de sucesso do matching, mas ainda nao garante 100%.

Nesse cenario, o email tende a vir quando:

- o evento do Outlook/Microsoft 365 lista os convidados individualmente;
- a Recall esta sincronizando esse calendario;
- o nome mostrado pelo participante no Teams bate de forma suficiente com o attendee do calendario.

Mesmo assim, o email pode vir `null` quando:

- o participante entra com nome diferente;
- o convite foi enviado para um grupo/lista e nao para pessoas individuais;
- o calendario nao expoe todos os attendees;
- existem nomes ambiguos;
- o matching ficou com baixa confianca.

### Configuracao necessaria

Pela documentacao da Recall, o requisito principal nao e uma configuracao especial no tenant do Teams para "liberar email na call". O requisito real esta na combinacao abaixo:

- Calendar Integration ativa;
- feature de participant emails habilitada pela Recall;
- qualidade boa dos attendees no calendario.

Em resumo:

- nome do participante: sim, normalmente disponivel;
- email do participante: pode vir, mas por matching;
- mesma organizacao ajuda, mas nao basta sozinha;
- a plataforma precisa lidar com `null`, baixa confianca e eventual reconciliacao posterior.

## Arquitetura sugerida

### POC

1. Criar um email/agenda dedicados.
2. Conectar essa agenda na Recall.
3. Convidar esse email para as reunioes que devem ser capturadas.
4. Agendar o bot no evento sincronizado.
5. Receber webhooks.
6. Buscar transcript, audio, metadata e participant events ao final.
7. Processar internamente.

Essa arquitetura e suficiente para validar:

- entrada automatica na reuniao;
- disponibilidade de transcript e audio;
- qualidade de metadata;
- qualidade de identificacao de participantes.

### Uso em escala

Em uso em escala, o desenho deveria evoluir para uma arquitetura com mais controle operacional:

1. Camada interna de sync de eventos e estado, com tabela propria de meetings, bots, recordings e participants.
2. Regras de negocio para decidir quais eventos devem ou nao ser gravados.
3. Mapeamento entre evento interno, evento da Recall e bot da Recall.
4. Processamento assíncrono dos webhooks e dos artefatos finais.
5. Retention/delecao automatica dos dados da Recall apos ingestao e persistencia interna.
6. Camada de reconciliacao de participantes para tratar emails nulos, ambiguos ou incorretos.
7. Observabilidade de lifecycle, falhas de entrada, falhas de gravacao e falhas de matching de identidade.

Comparado com a POC, o ponto principal da arquitetura de escala e que a Recall deixa de ser apenas uma camada de captura e passa a ser um sistema integrado ao nosso controle operacional e de identidade.

## O que testar primeiro

### Fluxo minimo obrigatorio

1. Agendar reuniao via calendario.
2. Confirmar criacao/agendamento do bot.
3. Iniciar e finalizar uma gravacao.
4. Buscar transcript.
5. Buscar meeting metadata.
6. Buscar participant events.
7. Apagar media depois do processamento.

### Endpoints mais importantes

#### Gestao de bot e agendamento

- `POST /api/v1/bot/`
- `GET /api/v1/bot/{bot_id}/`
- `DELETE /api/v1/bot/{bot_id}/`
- `POST /api/v1/bot/{bot_id}/leave_call/`
- `POST /api/v2/calendar-events/{id}/bot/`
- `DELETE /api/v2/calendar-events/{id}/bot/`

#### Gravacao e artefatos

- `POST /api/v1/bot/{bot_id}/start_recording/`
- `POST /api/v1/bot/{bot_id}/stop_recording/`
- `GET /api/v1/recording/{recording_id}/`
- `GET /api/v1/transcript/{transcript_id}/`
- `GET /api/v1/audio_mixed/{audio_id}/`
- `GET /api/v1/meeting_metadata/{metadata_id}/`
- `GET /api/v1/participant_events/{event_id}/`
- `POST /api/v1/bot/{bot_id}/delete_media/`

#### Webhooks

- `bot.joined`
- `recording.started`
- `recording.done`
- `bot.left`

## Pontos importantes que a POC precisa deixar claros

### O que a Recall resolve bem

- entrada em multiplas plataformas por API unificada;
- captura de transcript, audio, video e metadata;
- operacao com calendar lifecycle;
- diarizacao e recursos para separar participantes.

### O que continua sendo responsabilidade da nossa plataforma

- decidir quais reunioes devem ser capturadas;
- gerenciar consentimento e regras de negocio;
- manter mapeamento entre reuniao interna, evento de calendario e bot;
- tratar identidade incompleta dos participantes;
- processar e armazenar os artefatos finais.

### Limitacoes relevantes

- email do participante nao e garantido;
- bots criados fora da integracao de calendario nao recebem email derivado dos convidados;
- se quisermos evitar duplicacao de bots fora do calendar integration, a deduplicacao e nossa;
- "deletar reuniao" na pratica significa cancelar bot e/ou apagar media, nao apagar o evento do provedor.

## Funcionalidades opcionais para fase 2

- `Perfect Diarization` para maior precisao de "quem falou o que";
- audio separado por participante para processamento proprio;
- camada interna de reconciliacao de participantes quando email vier `null`;
- automacao de retention e delecao de media apos ingestao.

## Conclusao

A Recall.ai faz sentido para esta POC se o objetivo for capturar reunioes de forma confiavel, com transcript e metadata por API, deixando o processamento e a inteligencia de produto do nosso lado.

O desenho mais coerente e operar com:

- agenda dedicada;
- calendar integration;
- bots agendados;
- webhooks para lifecycle;
- ingestao interna de transcript, audio e metadata;
- tratamento proprio de identidade dos participantes quando o email nao vier preenchido.
