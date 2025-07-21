task "busybox" {
    driver = "litegix-fc"

    config {
        image = "docker.io/library/busybox:latest"
        snapshotter = "devmapper"
        vpu_count = 1
        mem_size = 256


        args = ["/bin/sh", "-c", "echo 'Hello, World!' && sleep 10"]
    }


}