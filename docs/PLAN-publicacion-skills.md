# Plan: publicación de learnings como skills por-proyecto

**Arquitectura "índice + skills temáticas"**

> Estado: propuesta de diseño. Autor original: sesión de trabajo con el proyecto
> padreseducadores.org (2026-07-12). Verificado contra el comportamiento real del
> binario royo-learn v0.1.6 (el pipeline exige `approved` antes de publicar).

---

## 1. Objetivo

Convertir los learnings curados de royo-learn en **conocimiento que el agente consume
solo en cada sesión**, sin inflar `AGENTS.md`/`CLAUDE.md` ni crear una skill por cada
captura. `AGENTS.md` se referencia **una sola vez** a una skill índice estable; el resto
del conocimiento vive en skills temáticas que el agente descubre bajo demanda por su
`description`/trigger.

## 2. Motivación

El problema central de toda memoria de agente es que **el contexto es finito**: no se
puede inyectar todo. Con cientos o miles de learnings, hace falta un mecanismo de
selección. Solo tres superficies entran al modelo automáticamente por sesión:

1. El system prompt (incluye `AGENTS.md`/`CLAUDE.md`).
2. Las skills (se cargan por su trigger/`description`).
3. La memoria auto-inyectada por hooks (p. ej. Engram).

Un learning en estado `captured` **no** llega solo al agente: es `pull` (hay que buscarlo
con `search_learnings`). Su valor se materializa al **publicarlo** a una skill o a
`AGENTS.md` — superficies que el agente sí lee sin buscar.

## 3. Decisiones de diseño (correcciones sobre la idea inicial)

La idea base —`AGENTS.md` estable + skill "madre" como índice + skills hijas por
conocimiento— es correcta. Se ajusta en tres puntos:

1. **Una skill por ÁREA, no por captura.** Si cada learning crea una skill, se produce
   una explosión de micro-skills; como el agente carga la `description` de todas para
   elegir, se reintroduce el problema de escala. Cada captura nueva **se acumula/actualiza
   dentro** de la skill temática correspondiente. Skill nueva solo cuando aparece un área
   nueva.
2. **La publicación no es automática.** El pipeline de royo-learn ya exige
   `captured → approved → publish` (verificado: `preview_publication` falla con
   `learning must be approved`). El gate de curación es el control de calidad que evita
   llenar las skills de ruido. Se automatiza el **borrador** (preview), no el **OK final**.
3. **El nombre de la skill es por dominio, no por herramienta.** Al agente le importa
   *cuándo* usar la skill (dominio), no que la generó royo-learn. La trazabilidad de origen
   va en el **frontmatter** (`source`, `learning_ids`), no en el nombre.

## 4. Modelo de 3 capas (regla de oro: guardar mucho, publicar poco)

1. **Siempre presente** — `AGENTS.md`/`CLAUDE.md`: una línea estable que apunta a la skill
   índice del proyecto. No cambia nunca.
2. **Activado por contexto** — skills temáticas del proyecto (`<proyecto>-<area>`),
   seleccionadas por su `description`/trigger.
3. **Reservorio bajo demanda** — base de royo-learn (`.royo-learn/royo-learn.db`): todos
   los learnings, buscables con `search_learnings`. Solo los aprobados y publicados suben a
   la capa 2.

## 5. Arquitectura

```
AGENTS.md / CLAUDE.md
  └─ (1 línea estable) → skill índice:  <proyecto>-conocimiento   [la "madre"]
        ├─ <proyecto>-dashboard-datos        (skill temática / hija)
        ├─ <proyecto>-n8n-selector           (skill temática / hija)
        └─ <proyecto>-mis-hijos              (skill temática / hija)
              ▲
              └─ cada hija se genera/actualiza desde learnings APROBADOS de royo-learn
```

## 6. Convenciones

- **Skill índice (madre):** `<proyecto>-conocimiento`. Su cuerpo es un catálogo: lista cada
  skill hija con **cuándo usarla** (1 línea de trigger). Se regenera automáticamente al
  publicar/actualizar cualquier hija.
- **Skills hijas:** `<proyecto>-<area>` (dominio, no herramienta).
  Ej.: `padreseducadores-dashboard-datos`.
- **Frontmatter obligatorio de cada hija:**

  ```yaml
  name: padreseducadores-dashboard-datos
  description: "Trigger: fechas/unidades/tests de dashboard_data_cursos, distribución de calendario del profesor, variantes profe_*_ec_*. Reglas y anti-patrones de datos del dashboard."
  source: royo-learn
  project: padreseducadores.org
  learning_ids: [019f588c-0861-7350-bf36-87d7b74d91d0]
  updated_at: 2026-07-12
  ```

- **Trazabilidad:** el origen (`royo-learn`) y los `learning_ids` viven en el frontmatter,
  no en el nombre.

## 7. Flujo end-to-end

1. **Capture** (`capture_learning`) → estado `captured`. Junta candidatos, sin publicar.
2. **Curate** (`curate_learning: approve`) → estado `approved`. Punto de control de calidad.
   Política sugerida:
   - **Auto-approve** permitido solo si `evidence_level=strong` **y** `confidence=high`
     **y** sin conflicto con un learning existente.
   - En cualquier otro caso, **gate humano**.
3. **Resolver destino (área):** decidir a qué skill temática pertenece el learning (por
   `retrieval_terms`/área). Si no existe esa área → crear skill hija nueva; si existe →
   **actualizarla** (acumular, no duplicar).
4. **Preview** (`preview_publication`) → muestra el archivo de skill que se crearía/modificaría
   y el diff del índice.
5. **Publish** (`publish_learning`, destino `skill`) → genera/actualiza el archivo de la skill
   hija **y** regenera el catálogo de la skill índice. Idempotente: re-publicar actualiza, no
   duplica.

## 8. Qué hay que implementar en royo-learn

> Hoy existen los estados (`captured/approved`), los destinos (`skill`, `agents_rule`) y el
> gate de aprobación. Falta el **generador de skills** y el **índice**:

1. **Publisher a skill:** dado un learning `approved` con destino `skill`, generar/actualizar
   un archivo de skill con frontmatter válido (`name`, `description` con triggers derivados de
   `retrieval_terms`, cuerpo con regla + procedimiento + ejemplo canónico + anti-patrón +
   límites).
2. **Política de agrupación por área:** mapear learning → skill temática (por
   área/`retrieval_terms`). Regla: **actualizar** la skill del área si existe; **crear** solo
   si es un área nueva. Nunca "una skill por learning".
3. **Generador de índice (skill madre):** mantener `<proyecto>-conocimiento` como catálogo
   autogenerado de las hijas (nombre + cuándo usarla), actualizado en cada publish.
4. **Idempotencia y merge:** re-publicar un learning ya publicado actualiza su sección dentro
   de la skill; publicar otro learning de la misma área lo **agrega** a la misma skill.
5. **Enganche a `AGENTS.md`/`CLAUDE.md`:** insertar (una única vez) la línea estable que
   referencia la skill índice. No volver a tocar ese archivo nunca más.

## 9. Anti-patrones a evitar

- ❌ Una skill por cada captura → explosión de micro-skills.
- ❌ Auto-publicar sin curación → ruido en lo que el agente lee siempre.
- ❌ Prefijo funcional (`royo-learn-…`) en el nombre → el agente necesita dominio, no
  herramienta.
- ❌ Duplicar en Engram lo ya publicado a skills → una sola fuente canónica
  (royo-learn/skill); Engram queda para memoria operativa de sesión.

## 10. Criterio de éxito

- `AGENTS.md` cambia **una vez** (la línea a la skill índice) y nunca más.
- El número de skills crece con las **áreas** del proyecto, no con las capturas.
- Cualquier agente, al entrar al proyecto, ve la skill índice y sabe dónde está cada
  conocimiento — sin que nadie edite el prompt.
