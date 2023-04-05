# tautulli-rich-presence

This is a bridge to allow a Tautulli webhook to be invoked and propagated as a Discord rich presence.

## Setting Up Tautulli Webhook

This expects a webhook payload that conforms to the following structure:

```json
{
    "title": "{title}",
    "actors": "{actors}",
    "studio": "{studio}",
    "secondsRemaining": "{remaining_duration_sec}"
}
```

This service listens for requests on the `/` path at port 9843.

## Running the Program

Execute the program, with the first parameter given being the Discord application ID you've registered in the Developer Portal for this application to use.