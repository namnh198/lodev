#lodev-generated

Files in this directory will be used to customize the WebImage, you can add:

- .lodev/web-build/Dockerfile
- .lodev/web-build/Dockerfile.\*

Additionaly, you can add the `pre-` variants that are inserted before what LODEV adds:

- .lodev/web-build/pre-Dockerfile
- .lodev/web-build/pre-Dockerfile.\*

Finally, you can also use `prepend.` variants that are inserted on top of the Dockerfile allowing for Multi-stage builds and other more complex use cases:

- .lodev/web-build/prepend.Dockerfile
- .lodev/web-build/prepend.Dockerfile.\*

See https://docs.docker.com/build/building/multi-stage/
