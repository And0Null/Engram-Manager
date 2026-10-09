# Feature: Engram Manager — TUI en Go

<!-- nodd:slug engram-manager-tui -->

## Objective

Construir una TUI propia en Go/Bubbletea en ~/Projects/lab/Engram-Manager para ver, buscar, editar, borrar y auditar las memorias de Engram, más métricas de salud y panel de sesiones.

## Problem

`engram tui` existe pero es limitado y no expone métricas de salud ni un panel de sesiones usable; no hay forma cómoda de navegar el corpus (25 MB + WAL) ni de editar/borrar memorias de forma visual. El resultado son 3 warnings de `engram doctor` (sesiones ambiguas, drift de proyecto) que nadie ve.

## Scope

Cubre: lista y detalle de memorias con filtros (proyecto/tipo/scope), búsqueda FTS, timeline de una memoria, create/edit/soft-delete/hard-delete, duplicar, panel de métricas+doctor, panel de sesiones/actividad. Fuera de alcance: sincronización con la nube, resolución automática de conflictos, escritura de memorias desde la TUI más allá de CRUD, GUI web (fase posterior si el núcleo funciona).

## Constraints

Sin dependencias nuevas innecesarias; solo lectura sobre `~/.engram/engram.db` salvo CRUD explícito del usuario; nunca mutar la DB mientras engram está escribiendo (respetar WAL); toda operación destructiva requiere confirmación visible en la TUI.

## Route

- intent: change
- route: tracked

## Verification

- runner: go test ./...
- tdd: off
- source: nodd_declare
- files: none declared

## Delivery

- strategy: ask-on-risk
- chain: stacked-to-main
- forecast: 0
- running: unknown

## Tasks



## Outcome

No tasks declared yet.

## Progress


