- [ ] controllo handleMyElection se ha senso chiamarla con go
- [ ] implemento lastLogIndex, lastLogTerm
- [ ] aggiungo *log.Fatal* in caso di mancata connessione riuscita all'avvio

- [ ] riguardo logica candidate riprova votazione con time.after o ticker

- [x] implement *heartbeat* logic
- [ ] implement *log* struct

# GOOD POINTS
- per ogni termine invieremo al massimo un voto per nodo registrato all'interno del channel *voteResponseCh*
- le richieste di voto avvengono fino a quando non riceviamo una risposta valida (non per forza positiva) da un nodo, introducendo
    uno time.Sleep per evitare flooding del nodo ricevente

# POSSIBILI MIGLIORAMENTI
- maggiori controlli sui channel, controllo errori se sono aperti