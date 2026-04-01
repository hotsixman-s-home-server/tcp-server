const socket = await Bun.connect({
    hostname: "localhost",
    port: 3000,
    socket: {
        open(socket) {
            socket.write(JSON.stringify({ name: "bar", key: "bar" }) + '\n')
        },
        data(socket, buffer) {
            console.log(buffer.toString('utf-8'))
        },
        error(_, error) {
            console.error(error)
        },
        close() {
            console.warn('closed')
        }
    }
})

setInterval(() => {
    socket.write(JSON.stringify({ destination: 'foo', id: 'asdasd' }) + '\n');
    socket.write(JSON.stringify("fuck you") + '\n');
    socket.write(JSON.stringify(Date.now()) + '\n')
    socket.write('\0\n');
}, 1000)

export { }