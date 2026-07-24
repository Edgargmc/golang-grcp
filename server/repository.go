package main

import (
	"sync"

	pb "grpc-todo/gen"
)

// TodoRepository es todo lo que todoServer necesita saber sobre "dónde
// viven los datos": nada de gRPC, nada de validación, nada de eventos.
// Solo guardar y recuperar TodoItems.
type TodoRepository interface {
	// Insert guarda un nuevo item y le asigna un Id (el repositorio decide
	// cómo: autoincrement en memoria, AUTOINCREMENT de SQL, etc.).
	Insert(item *pb.TodoItem) error
	FindByID(id int32) (*pb.TodoItem, bool)
	FindAll() []*pb.TodoItem
	// Save persiste los cambios hechos a un item ya existente (item.Id
	// debe estar seteado).
	Save(item *pb.TodoItem) error
	// Delete elimina el item y devuelve su último estado, para poder
	// publicarlo en un TodoEvent antes de que desaparezca.
	Delete(id int32) (*pb.TodoItem, bool)
}

// inMemoryRepository es exactamente la misma lógica que antes vivía
// directamente adentro de todoServer (mapa + mutex + contador).
type inMemoryRepository struct {
	mu     sync.Mutex
	todos  map[int32]*pb.TodoItem
	nextID int32
}

func newInMemoryRepository() *inMemoryRepository {
	return &inMemoryRepository{
		todos:  make(map[int32]*pb.TodoItem),
		nextID: 1,
	}
}

func (r *inMemoryRepository) Insert(item *pb.TodoItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	item.Id = r.nextID
	r.nextID++
	r.todos[item.Id] = item

	return nil
}

func (r *inMemoryRepository) FindByID(id int32) (*pb.TodoItem, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.todos[id]
	return item, ok
}

func (r *inMemoryRepository) FindAll() []*pb.TodoItem {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]*pb.TodoItem, 0, len(r.todos))
	for _, item := range r.todos {
		items = append(items, item)
	}

	return items
}

func (r *inMemoryRepository) Save(item *pb.TodoItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.todos[item.Id] = item
	return nil
}

func (r *inMemoryRepository) Delete(id int32) (*pb.TodoItem, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.todos[id]
	if !ok {
		return nil, false
	}
	delete(r.todos, id)

	return item, true
}
