smog
====

Smog is an incremental backup tool for Tahoe-LAFS. It will watch a directory for changes and upload files to your tahoe grid. When you delete a file, smog won't touch your file so that you have time to restore it. Until your leases run out, you are able to restore the file.

__NOTE__: Smog was written as a way to explore the Go language and is sure to have many bugs. Do not rely on smog to store sensative information.

Quickstart
-----------

Build smog and run `smog start`. The program will start watching the current directory for any changes. That's it! When you want smog to stop making incremental updates just run `smog stop`.

To restore a file: `smog restore <file>`

To renew leases: `smog renew`

__NOTE__: When a file is deleted, smog assumes you are no longer interested in that file. Therefore smog will not renew its leases even though it retains information on the file. Currently there is no way to renew leases on deleted files through smog, but you are able to do so through other tahoe interfaces.
